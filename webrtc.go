package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

const (
	frameRate = 30
)

type Peer struct {
	frameProviderFactory           FrameProviderFactory
	frameProvider                  FrameProvider
	webrtcConn                     *webrtc.PeerConnection
	gatheringComplete              <-chan struct{}
	videoTrack                     *webrtc.TrackLocalStaticSample
	iceCandidates                  []webrtc.ICECandidateInit
	iceConnectionStateConnected    sync.Once
	iceConnectionStateDisconnected sync.Once
}

func NewPeer(frameProviderFactory FrameProviderFactory, webrtcConfig *webrtc.Configuration) (*Peer, error) {
	conn, err := webrtc.NewPeerConnection(*webrtcConfig)
	if err != nil {
		return nil, err
	}

	peer := Peer{
		frameProviderFactory: frameProviderFactory,
		webrtcConn:           conn,
		gatheringComplete:    webrtc.GatheringCompletePromise(conn),
	}

	conn.OnConnectionStateChange(peer.onConnectionStateChange)
	conn.OnICECandidate(peer.onICECandidate)
	conn.OnICEConnectionStateChange(peer.onICEConnectionStateChange)

	return &peer, nil
}

func (p *Peer) Open() error {
	capability := webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeVP8,
	}
	videoTrack, err := webrtc.NewTrackLocalStaticSample(capability, "video", "pion")
	if err != nil {
		return err
	}
	p.videoTrack = videoTrack

	if _, err := p.webrtcConn.AddTrack(videoTrack); err != nil {
		return err
	}

	offer, err := p.webrtcConn.CreateOffer(nil)
	if err != nil {
		return err
	}

	if err := p.webrtcConn.SetLocalDescription(offer); err != nil {
		return err
	}

	return nil
}

func (p *Peer) Close() error {
	return p.webrtcConn.Close()
}

func (p *Peer) GetICECandidates() []webrtc.ICECandidateInit {
	<-p.gatheringComplete
	return append([]webrtc.ICECandidateInit(nil), p.iceCandidates...)
}

func (p *Peer) GetOffer() *webrtc.SessionDescription {
	return p.webrtcConn.LocalDescription()
}

func (p *Peer) SetAnswer(answer *webrtc.SessionDescription) error {
	return p.webrtcConn.SetRemoteDescription(*answer)
}

func (p *Peer) onConnectionStateChange(s webrtc.PeerConnectionState) {
	fmt.Printf("Peer Connection State has changed: %s\n", s.String())

	if s == webrtc.PeerConnectionStateFailed {
		os.Exit(1)
	}
}

func (p *Peer) onICECandidate(iceCandidate *webrtc.ICECandidate) {
	if iceCandidate == nil {
		return
	}

	fmt.Printf("ICE Candidate has been received: %s\n", iceCandidate.String())

	p.iceCandidates = append(p.iceCandidates, iceCandidate.ToJSON())
}

func (p *Peer) onICEConnectionStateChange(connectionState webrtc.ICEConnectionState) {
	fmt.Printf("Connection State has changed: %s\n", connectionState.String())

	switch connectionState {
	case webrtc.ICEConnectionStateConnected:
		p.iceConnectionStateConnected.Do(func() {
			frameProvider, err := p.frameProviderFactory.NewFrameProvider()
			if err != nil {
				log.Panic(err)
			}
			p.frameProvider = frameProvider

			go func() {
				if err := p.writeSamples(); err != nil {
					log.Print(err)
				}
			}()
		})

	case webrtc.ICEConnectionStateDisconnected:
		p.iceConnectionStateDisconnected.Do(func() {
			if err := p.frameProvider.Close(); err != nil {
				log.Print(err)
			}
		})
	}
}

func (p *Peer) writeSamples() error {
	for {
		frame, err := p.frameProvider.Frame()
		if err != nil {
			return err
		}

		encoder, err := NewVP8Encoder(frame.Rect.Size(), frameRate)
		if err != nil {
			return err
		}

		data, err := encoder.Encode(frame)
		if err != nil {
			return err
		}

		sample := media.Sample{
			Data:     data,
			Duration: time.Second / time.Duration(frameRate),
		}

		if err := p.videoTrack.WriteSample(sample); err != nil {
			return err
		}

		time.Sleep(sample.Duration)
	}
}
