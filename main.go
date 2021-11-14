package main

import (
	"errors"
	"log"
)

func main() {
	room, err := NewRoom()
	if err != nil {
		log.Panic(err)
	}

	ice, err := room.PostICE()
	if err != nil {
		log.Panic(err)
	}

	if err := room.SendRegister(); err != nil {
		log.Panic(err)
	}

	peer, err := NewPeer(&VNCFrameProviderFactory{}, ice)
	if err != nil {
		log.Panic(err)
	}
	defer peer.Close()

	if err := peer.Open(); err != nil {
		log.Panic(err)
	}

	if err := room.PostOffer(peer.GetOffer()); err != nil {
		log.Panic(err)
	}

	for _, candidate := range peer.GetICECandidates() {
		if err := room.PostCandidate(&candidate); err != nil {
			log.Panic(err)
		}
	}

	log.Println(room.GetLink())

	answer, err := room.RecvAnswer()
	if err != nil {
		log.Panic(err)
	}

	if err := peer.SetAnswer(answer); err != nil {
		log.Panic(err)
	}

	for err := errors.New(""); err != nil; err = room.RecvBye() {
		// wait for bye message
	}

	if err := room.SendBye(); err != nil {
		log.Panic(err)
	}

	if err := room.PostLeave(); err != nil {
		log.Panic(err)
	}

	if err := room.Close(); err != nil {
		log.Panic(err)
	}
}
