package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"

	"github.com/lunixbochs/struc"
)

const magicNumber uint32 = 0x55aa55aa

const (
	SendFilePacketType            uint32 = 0x99
	OpenPacketType                uint32 = 0x01
	HeartbeatPacketType           uint32 = 0xaa
	ManufacturerInfoPacketType    uint32 = 0x14
	CarPlayPacketType             uint32 = 0x08
	SoftwareVersionPacketType     uint32 = 0xcc
	BluetoothAddressPacketType    uint32 = 0x0a
	BluetoothPINPacketType        uint32 = 0x0c
	PluggedPacketType             uint32 = 0x02
	UnpluggedPacketType           uint32 = 0x04
	VideoDataPacketType           uint32 = 0x06
	AudioDataPacketType           uint32 = 0x07
	TouchPacketType               uint32 = 0x05
	BluetoothDeviceNamePacketType uint32 = 0x0d
	WifiDeviceNamePacketType      uint32 = 0x0e
	BluetoothPairedListPacketType uint32 = 0x12
)

var messageTypes = map[reflect.Type]uint32{
	reflect.TypeOf(&SendFile{}):            0x99,
	reflect.TypeOf(&Open{}):                0x01,
	reflect.TypeOf(&Heartbeat{}):           0xaa,
	reflect.TypeOf(&ManufacturerInfo{}):    0x14,
	reflect.TypeOf(&CarPlay{}):             0x08,
	reflect.TypeOf(&SoftwareVersion{}):     0xcc,
	reflect.TypeOf(&BluetoothAddress{}):    0x0a,
	reflect.TypeOf(&BluetoothPIN{}):        0x0c,
	reflect.TypeOf(&Plugged{}):             0x02,
	reflect.TypeOf(&Unplugged{}):           0x04,
	reflect.TypeOf(&VideoData{}):           0x06,
	reflect.TypeOf(&AudioData{}):           0x07,
	reflect.TypeOf(&Touch{}):               0x05,
	reflect.TypeOf(&BluetoothDeviceName{}): 0x0d,
	reflect.TypeOf(&WifiDeviceName{}):      0x0e,
	reflect.TypeOf(&BluetoothPairedList{}): 0x12,
}

// Header is header structure of data protocol
type Header struct {
	Magic  uint32 `struc:"uint32,little"`
	Length uint32 `struc:"uint32,little"`
	Type   uint32 `struc:"uint32,little"`
	TypeN  uint32 `struc:"uint32,little"`
}

func packPayload(buffer io.Writer, payload interface{}) error {
	if reflect.ValueOf(payload).Elem().NumField() > 0 {
		return struc.Pack(buffer, payload)
	}
	// Nothing to do
	return nil
}

func packHeader(payload interface{}, buffer io.Writer, data []byte) error {
	msgType, found := messageTypes[reflect.TypeOf(payload)]
	if !found {
		return errors.New("No message found")
	}
	msgTypeN := (msgType ^ 0xffffffff) & 0xffffffff
	msg := &Header{Magic: magicNumber, Length: uint32(len(data)), Type: msgType, TypeN: msgTypeN}
	err := struc.Pack(buffer, msg)
	if err != nil {
		return err
	}
	_, err = buffer.Write(data)
	return err
}

func Marshal(payload interface{}) ([]byte, error) {
	var buf, buffer bytes.Buffer
	err := packPayload(&buf, payload)
	if err != nil {
		return nil, err
	}
	err = packHeader(payload, &buffer, buf.Bytes())
	return buffer.Bytes(), err
}

func GetPayloadByHeader(hdr Header) interface{} {
	switch hdr.Type {
	case SendFilePacketType:
		return new(SendFile)
	case OpenPacketType:
		return new(Open)
	case HeartbeatPacketType:
		return new(Heartbeat)
	case ManufacturerInfoPacketType:
		return new(ManufacturerInfo)
	case CarPlayPacketType:
		return new(CarPlay)
	case SoftwareVersionPacketType:
		return new(SoftwareVersion)
	case BluetoothAddressPacketType:
		return new(BluetoothAddress)
	case BluetoothPINPacketType:
		return new(BluetoothPIN)
	case PluggedPacketType:
		return new(Plugged)
	case UnpluggedPacketType:
		return new(Unplugged)
	case VideoDataPacketType:
		return new(VideoData)
	case AudioDataPacketType:
		return new(AudioData)
	case TouchPacketType:
		return new(Touch)
	case BluetoothDeviceNamePacketType:
		return new(BluetoothDeviceName)
	case WifiDeviceNamePacketType:
		return new(WifiDeviceName)
	case BluetoothPairedListPacketType:
		return new(BluetoothPairedList)
	}
	return &Unknown{Type: hdr.Type}
}

func UnmarhalVideoData(data []byte) (VideoData, error) {
	if len(data) < 20 {
		return VideoData{}, errors.New("wrong videodata size (<20)")
	}
	length := int32(binary.LittleEndian.Uint32(data[12:]))
	if len(data) != 20+int(length) {
		return VideoData{}, errors.New("wrong videodata size (len(data) != 20+length)")
	}
	return VideoData{
		Width:    int32(binary.LittleEndian.Uint32(data[0:])),
		Height:   int32(binary.LittleEndian.Uint32(data[4:])),
		Flags:    int32(binary.LittleEndian.Uint32(data[8:])),
		Length:   length,
		Unknown2: int32(binary.LittleEndian.Uint32(data[16:])),
		Data:     data[20:],
	}, nil
}

func float32FromBytes(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)
	float := math.Float32frombits(bits)
	return float
}

func UnmarshalAudioData(data []byte) (AudioData, error) {
	if len(data) < 12 {
		return AudioData{}, errors.New("wrong audiodata size (<12)")
	}

	switch len(data) - 12 {
	case 1:
		return AudioData{
			DecodeType: DecodeType(binary.LittleEndian.Uint32(data[0:])),
			Volume:     float32FromBytes(data[4:]),
			AudioType:  int32(binary.LittleEndian.Uint32(data[8:])),
			Command:    AudioCommand(data[12]),
		}, nil
	case 4:
		return AudioData{
			DecodeType:     DecodeType(binary.LittleEndian.Uint32(data[0:])),
			Volume:         float32FromBytes(data[4:]),
			AudioType:      int32(binary.LittleEndian.Uint32(data[8:])),
			VolumeDuration: int32(binary.LittleEndian.Uint32(data[12:])),
		}, nil
	default:
		return AudioData{
			DecodeType: DecodeType(binary.LittleEndian.Uint32(data[0:])),
			Volume:     float32FromBytes(data[4:]),
			AudioType:  int32(binary.LittleEndian.Uint32(data[8:])),
			Data:       data[12:],
		}, nil
	}
}

func UnmarshalHeader(data []byte) (Header, error) {
	if len(data) != 16 {
		return Header{}, errors.New("wrong header size (!=16)")
	}
	hdr := Header{
		Magic:  binary.LittleEndian.Uint32(data[0:]),
		Length: binary.LittleEndian.Uint32(data[4:]),
		Type:   binary.LittleEndian.Uint32(data[8:]),
		TypeN:  binary.LittleEndian.Uint32(data[12:]),
	}
	if hdr.Magic != magicNumber {
		return Header{}, errors.New("Invalid magic number")
	}
	if (hdr.Type^0xffffffff)&0xffffffff != hdr.TypeN {
		return Header{}, errors.New("Invalid type")
	}
	return hdr, nil
}

func Unmarshal(data []byte, payload interface{}) error {
	if len(data) > 0 {
		err := struc.Unpack(bytes.NewBuffer(data), payload)
		if err != nil {
			return err
		}
	}

	switch payload := payload.(type) {
	case *Header:
		if payload.Magic != magicNumber {
			return errors.New("Invalid magic number")
		}
		if (payload.Type^0xffffffff)&0xffffffff != payload.TypeN {
			return errors.New("Invalid type")
		}
	case *AudioData:
		switch len(data) - 12 {
		case 1:
			payload.Command = AudioCommand(data[12])
		case 4:
			binary.Read(bytes.NewBuffer(data[12:]), binary.LittleEndian, &payload.VolumeDuration)
		default:
			payload.Data = data[12:]
		}
	case *BluetoothDeviceName:
		payload.Data = NullTermString(data)
	case *WifiDeviceName:
		payload.Data = NullTermString(data)
	case *BluetoothPairedList:
		payload.Data = NullTermString(data)
	case *Unknown:
		payload.Data = data
	}

	return nil
}
