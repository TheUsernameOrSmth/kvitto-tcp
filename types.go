package main

import (
	"encoding/json"
	"time"
)

type Kvitto struct {
	Time          time.Time     `json:"-"`
	TargetPrinter string        `json:"target_printer"`
	Timestamp     int64         `json:"timestamp"`
	Sold          []SoldProduct `json:"sold"`
	SoldBy        string        `json:"sold_by"`
	IsBong        bool          `json:"is_bong"`
	Number        int           `json:"number"`
}
type Message struct {
	Message string `json:"message"`
}

func (m *Message) publish() {
	data, _ := json.Marshal(m)
	mq.Publish(mqttTopic+"/message", 0, false, data)
}

func (k *Kvitto) publish() {
	k.Timestamp = k.Time.Unix()
	data, _ := json.Marshal(k)
	mq.Publish(mqttTopic+"/kvitto", 0, false, data)
}

type SoldProduct struct {
	Category  string `json:"category"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
	ExtraText string `json:"extra_text"`
}
