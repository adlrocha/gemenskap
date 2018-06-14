package main

import (
	"bufio"
	"context"
	"fmt"
	"log"

	inet "gx/ipfs/QmXdgNhVEgjLxjUoMs5ViQL7pboAt3Y7V7eGHRiE4qrmTE/go-libp2p-net"

	pb "github.com/libp2p/go-libp2p/examples/multipro/pb"
	"gx/ipfs/QmQQGtcp6nVUrQjNsnU53YWV1q8fK1Kd9S7FEkYbRZzxry/go-libp2p-host"
	protobufCodec "gx/ipfs/QmRDePEiL4Yupq5EkcK3L3ko3iMgYaqUdLu7xc1kqs7dnV/go-multicodec/protobuf"
	uuid "gx/ipfs/QmcBWojPoNh4qm7zvv4qiepvCnnc7ALS9qcp7TNwwxT1gT/go.uuid"
)

// pattern: /protocol-name/request-or-response-message/version
const echoRequest = "/echo/echoreq/0.0.1"
const echoResponse = "/echo/echoresp/0.0.1"

type EchoProtocol struct {
	node     *Node                      // local host
	requests map[string]*pb.EchoRequest // used to access request data from response handlers
	done     chan bool                  // only for demo purposes to hold main from terminating
}

func NewEchoProtocol(node *Node, done chan bool) *EchoProtocol {
	e := EchoProtocol{node: node, requests: make(map[string]*pb.EchoRequest), done: done}
	node.SetStreamHandler(echoRequest, e.onEchoRequest)
	node.SetStreamHandler(echoResponse, e.onEchoResponse)

	// design note: to implement fire-and-forget style messages you may just skip specifying a response callback.
	// a fire-and-forget message will just include a request and not specify a response object

	return &e
}

// remote peer requests handler
func (e *EchoProtocol) onEchoRequest(s inet.Stream) {
	// get request data
	data := &pb.EchoRequest{}
	decoder := protobufCodec.Multicodec(nil).Decoder(bufio.NewReader(s))
	err := decoder.Decode(data)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("%s: Received echo request from %s. Message: %s", s.Conn().LocalPeer(), s.Conn().RemotePeer(), data.Message)

	valid := e.node.authenticateMessage(data, data.MessageData)

	if !valid {
		log.Println("Failed to authenticate message")
		return
	}

	log.Printf("%s: Sending echo response to %s. Message id: %s...", s.Conn().LocalPeer(), s.Conn().RemotePeer(), data.MessageData.Id)

	// send response to the request using the message string he provided

	resp := &pb.EchoResponse{
		MessageData: e.node.NewMessageData(data.MessageData.Id, false),
		Message:     data.Message}

	// sign the data
	signature, err := e.node.signProtoMessage(resp)
	if err != nil {
		log.Println("failed to sign response")
		return
	}

	// add the signature to the message
	resp.MessageData.Sign = string(signature)

	s, respErr := e.node.NewStream(context.Background(), s.Conn().RemotePeer(), echoResponse)
	if respErr != nil {
		log.Println(respErr)
		return
	}

	ok := e.node.sendProtoMessage(resp, s)

	if ok {
		log.Printf("%s: Echo response to %s sent.", s.Conn().LocalPeer().String(), s.Conn().RemotePeer().String())
	}
}

// remote echo response handler
func (e *EchoProtocol) onEchoResponse(s inet.Stream) {
	data := &pb.EchoResponse{}
	decoder := protobufCodec.Multicodec(nil).Decoder(bufio.NewReader(s))
	err := decoder.Decode(data)
	if err != nil {
		return
	}

	// authenticate message content
	valid := e.node.authenticateMessage(data, data.MessageData)

	if !valid {
		log.Println("Failed to authenticate message")
		return
	}

	// locate request data and remove it if found
	req, ok := e.requests[data.MessageData.Id]
	if ok {
		// remove request from map as we have processed it here
		delete(e.requests, data.MessageData.Id)
	} else {
		log.Println("Failed to locate request data boject for response")
		return
	}

	if req.Message != data.Message {
		log.Fatalln("Expected echo to respond with request message")
	}

	log.Printf("%s: Received echo response from %s. Message id:%s. Message: %s.", s.Conn().LocalPeer(), s.Conn().RemotePeer(), data.MessageData.Id, data.Message)
	e.done <- true
}

func (e *EchoProtocol) Echo(host host.Host) bool {
	log.Printf("%s: Sending echo to: %s....", e.node.ID(), host.ID())

	// create message data
	req := &pb.EchoRequest{
		MessageData: e.node.NewMessageData(uuid.Must(uuid.NewV4()).String(), false),
		Message:     fmt.Sprintf("Echo from %s", e.node.ID())}

	signature, err := e.node.signProtoMessage(req)
	if err != nil {
		log.Println("failed to sign message")
		return false
	}

	// add the signature to the message
	req.MessageData.Sign = string(signature)

	s, err := e.node.NewStream(context.Background(), host.ID(), echoRequest)
	if err != nil {
		log.Println(err)
		return false
	}

	ok := e.node.sendProtoMessage(req, s)

	if !ok {
		return false
	}

	// store request so response handler has access to it
	e.requests[req.MessageData.Id] = req
	log.Printf("%s: Echo to: %s was sent. Message Id: %s, Message: %s", e.node.ID(), host.ID(), req.MessageData.Id, req.Message)
	return true
}