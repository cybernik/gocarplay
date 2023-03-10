package usblink

import (
	"bufio"
	"github.com/google/gousb"
	"log"
	"time"
	"webrtc/protocol"
)

type USBLink struct {
	exitChan    chan struct{}
	outData     chan interface{}
	waitGroup   WaitGroupWrapper
	usbCtx      *gousb.Context
	onVideo     func(protocol.VideoData)
	onAudio     func(protocol.AudioData)
	onData      func(interface{})
	onError     func(error)
	onReadySend func()
}

func (l *USBLink) loop() {
	//stage 1: detect and connect
	timeAfter := 0 * time.Second //first time immediately
	var (
		product *gousb.Device
		err     error
	)
	for {
		select {
		case <-time.After(timeAfter):
			timeAfter = 2 * time.Second
		case <-l.exitChan:
			return
		}
		product, err = l.usbConnect()
		if err != nil {
			log.Printf("error occurred while discovering product: %s. next try after 2 seconds...\n", err)
		} else if product == nil {
			log.Println("product not found, next try after 2 seconds...")
		} else {
			break
		}
	}
	defer product.Close()

	intf, done, err := product.DefaultInterface()
	if err != nil {
		log.Fatal(err)
	}
	defer done()

	//TODO: найти in/out в устройстве
	epOut, err := intf.OutEndpoint(1)
	if err != nil {
		log.Fatal(err)
	}
	epIn, err := intf.InEndpoint(1)
	if err != nil {
		log.Fatal(err)
	}

	var endpointWg WaitGroupWrapper
	endpointWg.Wrap(func() {
		l.outEndpointProcess(epOut)
	})
	endpointWg.Wrap(func() {
		l.inEndpointProcess(epIn)
	})
	endpointWg.Wait()
}

func (l *USBLink) outEndpointProcess(out *gousb.OutEndpoint) {

	/*
		stream, err := out.NewStream(512*9600, 1)
		if err != nil {
			log.Fatal(err)
		}
		defer stream.Close()
	*/

	l.onReadySend()

	buff := make([]byte, 0, 512*9600)

	timeAfter := 2 * time.Second
	for {
		select {
		case <-time.After(timeAfter):
			//log.Println("herbeat")
			l.sendUsbMessage(out, &protocol.Heartbeat{})
		case msg := <-l.outData:
			start := time.Now()
			remaining := cap(buff)
			bMsg, err := protocol.Marshal(msg)
			if err != nil {
				log.Fatal(err)
			}
			if remaining > len(bMsg) {
				remaining -= len(bMsg)
				buff = append(buff, bMsg...)
			} else {
				_, err = out.Write(bMsg)
				if err != nil {
					log.Fatal(err)
				}
				continue
			}

		loop:
			for {
				if time.Now().Sub(start) > 300*time.Millisecond {
					_, err = out.Write(buff)
					if err != nil {
						log.Fatal(err)
					}
					break loop
				}
				select {
				case msg = <-l.outData:
					bMsg, err = protocol.Marshal(msg)
					if err != nil {
						log.Fatal(err)
					}
					if remaining > len(bMsg) {
						remaining -= len(bMsg)
						buff = append(buff, bMsg...)
					} else {
						_, err = out.Write(buff)
						if err != nil {
							log.Fatal(err)
						}

						_, err = out.Write(bMsg)
						if err != nil {
							log.Fatal(err)
						}
						break loop
					}
				default:
					_, err = out.Write(buff)
					if err != nil {
						log.Fatal(err)
					}
					break loop
				}
			}

			buff = buff[:0]
		case <-l.exitChan:
			return
		}
	}
}

func (l *USBLink) inEndpointProcess(in *gousb.InEndpoint) {
	//ctx := context.Background()

	stream, err := in.NewStream(512*9600, 180)
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	br := bufio.NewReaderSize(stream, 512*9600)

	//incoming := make(chan incomingPacket, 10000)
	//go l.incoming(incoming)

	for {
		select {
		case <-l.exitChan:
			return
		default:
			packet, err := l.receiveVideoAudioUsbMessage(br)
			if err != nil && l.onError != nil {
				l.onError(err)
			} else if packet.buf != nil && l.onData != nil {
				switch packet.header.Type {
				case protocol.VideoDataPacketType:
					video, err := protocol.UnmarhalVideoData(packet.buf)
					if err != nil && l.onError != nil {
						l.onError(err)
					} else {
						l.onVideo(video)
					}
				case protocol.AudioDataPacketType:
					audio, err := protocol.UnmarshalAudioData(packet.buf)
					if err != nil && l.onError != nil {
						l.onError(err)
					} else {
						l.onAudio(audio)
					}
				default:
					/*
						payload := protocol.GetPayloadByHeader(packet.data.header)
						err := protocol.Unmarshal(packet.data.buf, payload)
						if err != nil && l.onError != nil {
							l.onError(err)
						} else {
							switch data := payload.(type) {
							case *protocol.VideoData:
								incomingVideo <- *data
							case *protocol.AudioData:
								incomingAudio <- *data
							default:
								incomingData <- data
							}
						}
					*/
				}
			}

			/*
				incoming <- incomingPacket{
					data: received,
					err:  err,
				}
			*/

			/*
				select {
				case incoming <- incomingPacket{
					data: received,
					err:  err,
				}:
				default:
					log.Println("packet dropped!!!")
				}

			*/

		}
	}
}

type incomingPacket struct {
	data usbMessage
	err  error
}

func (l *USBLink) incoming(in chan incomingPacket) {
	incomingVideo := make(chan protocol.VideoData, 10000)
	go l.incomingVideo(incomingVideo)

	incomingAudio := make(chan protocol.AudioData, 10000)
	go l.incomingAudio(incomingAudio)

	incomingData := make(chan interface{}, 10000)
	go l.incomingData(incomingData)

	for {
		select {
		case <-l.exitChan:
			return
		case packet := <-in:
			if packet.err != nil && l.onError != nil {
				l.onError(packet.err)
			} else if packet.data.buf != nil && l.onData != nil {
				switch packet.data.header.Type {
				case protocol.VideoDataPacketType:
					video, err := protocol.UnmarhalVideoData(packet.data.buf)
					if err != nil && l.onError != nil {
						l.onError(err)
					} else {
						incomingVideo <- video
					}
				case protocol.AudioDataPacketType:
					audio, err := protocol.UnmarshalAudioData(packet.data.buf)
					if err != nil && l.onError != nil {
						l.onError(err)
					} else {
						incomingAudio <- audio
					}
				default:
					/*
						payload := protocol.GetPayloadByHeader(packet.data.header)
						err := protocol.Unmarshal(packet.data.buf, payload)
						if err != nil && l.onError != nil {
							l.onError(err)
						} else {
							switch data := payload.(type) {
							case *protocol.VideoData:
								incomingVideo <- *data
							case *protocol.AudioData:
								incomingAudio <- *data
							default:
								incomingData <- data
							}
						}
					*/
				}
			}
		}
	}
}

func (l *USBLink) incomingVideo(in chan protocol.VideoData) {
	//timeAfter := 1 * time.Second
	for {
		select {
		//case <-time.After(timeAfter):
		//	log.Println("no data 1 second")
		case <-l.exitChan:
			return
		case packet := <-in:
			l.onVideo(packet)
		}
	}
}

func (l *USBLink) incomingAudio(in chan protocol.AudioData) {
	for {
		select {
		case <-l.exitChan:
			return
		case packet := <-in:
			l.onAudio(packet)
		}
	}
}

func (l *USBLink) incomingData(in chan interface{}) {
	for {
		select {
		case <-l.exitChan:
			return
		case packet := <-in:
			l.onData(packet)
		}
	}
}

type usbMessage struct {
	header protocol.Header
	buf    []byte
}

func (l *USBLink) receiveVideoAudioUsbMessage(reader *bufio.Reader) (usbMessage, error) {
	for {
		msg, err := l.receiveUsbMessage(reader)
		if err != nil {
			return msg, err
		}
		if msg.header.Type == protocol.VideoDataPacketType || msg.header.Type == protocol.AudioDataPacketType {
			return msg, nil
		}
	}
}

func (l *USBLink) receiveUsbMessage(reader *bufio.Reader) (usbMessage, error) {
	buf := make([]byte, 16)

	num, err := reader.Read(buf)
	if err != nil || num != len(buf) {
		return usbMessage{}, err
	}
	hdr, err := protocol.UnmarshalHeader(buf[:num])
	if err != nil {
		return usbMessage{}, err
	}

	buf = make([]byte, hdr.Length)
	if hdr.Length > 0 {
		num, err = reader.Read(buf)
		if err != nil || num != len(buf) {
			return usbMessage{}, err
		}
	}

	return usbMessage{header: hdr, buf: buf}, nil
}

func (l *USBLink) sendUsbMessage(out *gousb.OutEndpoint, msg interface{}) error {
	buf, err := protocol.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = out.Write(buf)

	/*
		buf, err := protocol.Marshal(msg)
		if err != nil {
			return err
		}
		_, err = out.Write(buf[:16])
		if err != nil {
			return err
		}
		if len(buf) > 16 {
			_, err = out.Write(buf[16:])
		}
	*/
	return err
}

func (l *USBLink) usbConnect() (*gousb.Device, error) {
	vid, pid, pid2 := gousb.ID(0x1314), gousb.ID(0x1521), gousb.ID(0x1520)
	devs, err := l.usbCtx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		founded := desc.Vendor == vid && (desc.Product == pid || desc.Product == pid2)
		if founded {
			log.Printf("product found: %s", desc, desc.Speed)
			for _, cfgDesc := range desc.Configs {
				for _, intDesc := range cfgDesc.Interfaces {
					for _, altSetting := range intDesc.AltSettings {
						for endpointAddr, endpointDescr := range altSetting.Endpoints {
							log.Printf("%d/%d/%d/%s: %s\n", cfgDesc.Number, intDesc.Number, altSetting.Number, endpointAddr, endpointDescr)
						}
					}
				}
			}
		}
		return founded
	})
	if err != nil {
		return nil, err
	}
	if len(devs) > 0 {
		var device *gousb.Device
		for i, dev := range devs {
			if i == 0 {
				device = dev
			} else {
				dev.Close()
			}
		}
		return device, nil
	}
	return nil, nil
}

func (l *USBLink) SendMessage(msg interface{}) {
	//if l.outData != nil {
	l.outData <- msg
	//}
}

func (l *USBLink) Start(onReadySend func(), onVideo func(protocol.VideoData), onAudio func(protocol.AudioData), onData func(interface{}), onError func(error)) error {
	if l.exitChan != nil {
		return nil
	}

	l.onVideo = onVideo
	l.onAudio = onAudio
	l.onData = onData
	l.onError = onError
	l.onReadySend = onReadySend

	l.usbCtx = gousb.NewContext()
	l.exitChan = make(chan struct{})
	l.outData = make(chan interface{}, 1024)
	l.waitGroup.Wrap(l.loop)
	log.Println("USBLink started")
	return nil
}

func (l *USBLink) Stop() {
	if l.exitChan == nil {
		return
	}
	close(l.exitChan)
	l.waitGroup.Wait()
	l.exitChan = nil
	close(l.outData)
	l.outData = nil

	l.onData = nil
	l.onError = nil
	l.onReadySend = nil
	l.onVideo = nil
	l.onAudio = nil

	err := l.usbCtx.Close()
	if err != nil {
		log.Printf("USBLink stopped with error: %s\n", err)
	} else {
		log.Println("USBLink stopped")
	}
}
