package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

const (
	origin = "https://appr.tc"
)

func init() {
	rand.Seed(time.Now().Unix())
}

type JoinParams struct {
	ClientId     string `json:"client_id"`
	IceServerUrl string `json:"ice_server_url"`
	RoomId       string `json:"room_id"`
	RoomLink     string `json:"room_link"`
	WssPostUrl   string `json:"wss_post_url"`
	WssUrl       string `json:"wss_url"`
}

type Join struct {
	Params JoinParams `json:"params"`
	Result string     `json:"result"`
}

func postJoin() (*Join, error) {
	url := fmt.Sprintf("%s/join/%d", origin, 1e8+rand.Intn(9e8))

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var join Join
	if err := json.NewDecoder(res.Body).Decode(&join); err != nil {
		return nil, err
	}

	return &join, nil
}

type Room struct {
	params JoinParams
	wsConn *websocket.Conn
}

func NewRoom() (*Room, error) {
	join, err := postJoin()
	if err != nil {
		return nil, err
	}

	if result := join.Result; result != "SUCCESS" {
		return nil, errors.New(result)
	}

	conn, _, err := websocket.DefaultDialer.Dial(join.Params.WssUrl, http.Header{
		"Origin": []string{
			origin,
		},
	})
	if err != nil {
		return nil, err
	}

	room := Room{
		params: join.Params,
		wsConn: conn,
	}
	return &room, nil
}

var _ WebRTCConfigurationProvider = (*Room)(nil)

func (r *Room) WebRTCConfiguration() (*webrtc.Configuration, error) {
	return r.postICE()
}

func (r *Room) postICE() (*webrtc.Configuration, error) {
	req, err := http.NewRequest(http.MethodPost, r.params.IceServerUrl, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var ice webrtc.Configuration
	if err := json.NewDecoder(res.Body).Decode(&ice); err != nil {
		return nil, err
	}

	return &ice, nil
}

type Register struct {
	Cmd      string `json:"cmd"`
	RoomId   string `json:"roomid"`
	ClientId string `json:"clientid"`
}

func (r *Room) SendRegister() error {
	return r.wsConn.WriteJSON(Register{
		Cmd:      "register",
		RoomId:   r.params.RoomId,
		ClientId: r.params.ClientId,
	})
}

func (r *Room) postMessage(body []byte) error {
	url := fmt.Sprintf("%s/message/%s/%s", origin, r.params.RoomId, r.params.ClientId)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (r *Room) PostOffer(offer *webrtc.SessionDescription) error {
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return err
	}

	return r.postMessage(offerJSON)
}

type Candidate struct {
	Type      string `json:"type"`
	Label     uint16 `json:"label"`
	Id        string `json:"id"`
	Candidate string `json:"candidate"`
}

func (r *Room) PostCandidate(candidate *webrtc.ICECandidateInit) error {
	candidateJSON, err := json.Marshal(Candidate{
		Type:      "candidate",
		Label:     *candidate.SDPMLineIndex,
		Id:        *candidate.SDPMid,
		Candidate: candidate.Candidate,
	})
	if err != nil {
		return err
	}

	return r.postMessage(candidateJSON)
}

func (r *Room) GetLink() string {
	return r.params.RoomLink
}

type Message struct {
	Msg   string `json:"msg"`
	Error string `json:"error"`
}

type InnerMessage struct {
	Type string `json:"type"`
}

func (r *Room) RecvMsg(expectedType string) ([]byte, error) {
	_, buf, err := r.wsConn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var message Message
	if err := json.Unmarshal(buf, &message); err != nil {
		return nil, err
	}
	if err := message.Error; err != "" {
		return nil, errors.New(err)
	}
	msg := []byte(message.Msg)

	var innerMessage InnerMessage
	if err := json.Unmarshal(msg, &innerMessage); err != nil {
		return nil, err
	}
	if typ := innerMessage.Type; typ != expectedType {
		return nil, errors.New(typ)
	}

	return msg, nil
}

func (r *Room) RecvAnswer() (*webrtc.SessionDescription, error) {
	msg, err := r.RecvMsg("answer")
	if err != nil {
		return nil, err
	}

	var sdp webrtc.SessionDescription
	if err := json.Unmarshal(msg, &sdp); err != nil {
		return nil, err
	}

	return &sdp, nil
}

func (r *Room) RecvBye() error {
	_, err := r.RecvMsg("bye")
	return err
}

type Send struct {
	Cmd string `json:"cmd"`
	Msg string `json:"msg"`
}

func (r *Room) SendBye() error {
	byeJSON, err := json.Marshal(InnerMessage{
		Type: "bye",
	})
	if err != nil {
		return err
	}

	return r.wsConn.WriteJSON(Send{
		Cmd: "send",
		Msg: string(byeJSON),
	})
}

func (r *Room) PostLeave() error {
	url := fmt.Sprintf("%s/leave/%s/%s", origin, r.params.RoomId, r.params.ClientId)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (r *Room) Close() error {
	url := fmt.Sprintf("%s/%s/%s", r.params.WssPostUrl, r.params.RoomId, r.params.ClientId)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return r.wsConn.Close()
}
