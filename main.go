package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"webrtc/protocol"
	"webrtc/usblink"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type deviceSize struct {
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

type deviceTouch struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Action int32   `json:"action"`
}

var (
	videoTrack       *webrtc.TrackLocalStaticSample
	audioDataChannel *webrtc.DataChannel
	size             deviceSize
	fps              int32 = 30
	usbLink          *usblink.USBLink
)

func setupWebRTC(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	// WebRTC setup
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	mediaEngine := webrtc.MediaEngine{}

	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	stats, ok := pc.GetStats().GetConnectionStats(pc)
	if !ok {
		stats.ID = "unknoown"
	}

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("State of %s: %s \n", stats.ID, connectionState.String())
	})

	// Create a video track
	videoCodec := webrtc.RTPCodecCapability{
		MimeType:     webrtc.MimeTypeH264,
		ClockRate:    90000,
		Channels:     0,
		SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
		RTCPFeedback: nil,
	}
	if videoTrack, err = webrtc.NewTrackLocalStaticSample(videoCodec, "video", "video"); err != nil {
		return nil, err
	}

	if _, err = pc.AddTransceiverFromTrack(videoTrack,
		webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		},
	); err != nil {
		return nil, err
	}

	// Create a data channels
	audioDataChannel, err = pc.CreateDataChannel("audio", nil)
	if err != nil {
		return nil, err
	}

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		switch d.Label() {
		case "touch":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				sendTouch(msg.Data)
			})
		case "start":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				startCarPlay(msg.Data)
			})
		}
	})

	// Set the remote SessionDescription
	if err := pc.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create an answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	if err = pc.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	return &answer, nil
}

func webRTCOfferHandler(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	answer, err := setupWebRTC(offer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&answer)
}

func sendTouch(data []byte) {
	if usbLink != nil {
		var touch deviceTouch
		if err := json.Unmarshal(data, &touch); err != nil {
			return
		}
		usbLink.SendMessage(&protocol.Touch{X: uint32(touch.X * 10000 / float32(size.Width)), Y: uint32(touch.Y * 10000 / float32(size.Height)), Action: protocol.TouchAction(touch.Action)})
	}
}

func startCarPlay(data []byte) {
	if err := json.Unmarshal(data, &size); err != nil {
		return
	}

	usbLink = new(usblink.USBLink)
	usbLink.Start(func() {
		log.Println("device ready to init", size.Width, size.Height)
		initCarplay(size.Width, size.Height, fps, 160)
	}, func(data protocol.VideoData) {
		duration := time.Duration((float32(1) / float32(fps)) * float32(time.Second))
		videoTrack.WriteSample(media.Sample{Data: data.Data, Duration: duration})
	},
		func(data protocol.AudioData) {
			if len(data.Data) == 0 {
				//log.Printf("[onData] %#v", data)
			} else {
				var buf bytes.Buffer
				fr := protocol.AudioDecodeTypes[data.DecodeType].Frequency
				ch := protocol.AudioDecodeTypes[data.DecodeType].Channel
				binary.Write(&buf, binary.LittleEndian, fr)
				binary.Write(&buf, binary.LittleEndian, ch)
				audioDataChannel.Send(append(buf.Bytes(), data.Data...))
			}
		},
		func(data interface{}) {
			//log.Printf("[onData] %#v", data)
		}, func(err error) {
			log.Fatalf("[ERROR] %#v", err)
		})
}

func intToByte(data int32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, data)
	return buf.Bytes()
}

func initCarplay(width, height, fps, dpi int32) {
	usbLink.SendMessage(&protocol.SendFile{FileName: "/tmp/screen_dpi\x00", Content: intToByte(dpi)})
	usbLink.SendMessage(&protocol.Open{Width: width, Height: height, VideoFrameRate: fps, Format: 5, PacketMax: 4915200, IBoxVersion: 2, PhoneWorkMode: 2})

	usbLink.SendMessage(&protocol.ManufacturerInfo{A: 0, B: 0})
	usbLink.SendMessage(&protocol.SendFile{FileName: "/tmp/night_mode\x00", Content: intToByte(1)})
	usbLink.SendMessage(&protocol.SendFile{FileName: "/tmp/hand_drive_mode\x00", Content: intToByte(1)})
	usbLink.SendMessage(&protocol.SendFile{FileName: "/tmp/charge_mode\x00", Content: intToByte(0)})
	usbLink.SendMessage(&protocol.SendFile{FileName: "/tmp/box_name\x00", Content: bytes.NewBufferString("BoxName").Bytes()})
}

func main() {
	log.Println("http://localhost:8001")
	http.HandleFunc("/connect", webRTCOfferHandler)
	http.Handle("/", http.FileServer(http.Dir("./")))
	log.Fatal(http.ListenAndServe(":8001", nil))
}
