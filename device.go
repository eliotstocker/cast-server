package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"time"

	"github.com/bep/debounce"
	"github.com/stampzilla/gocast/handlers"
)

type ccApp struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type ccMedia struct {
	Title       string  `json:"title"`
	Subtitle    string  `json:"subtitle"`
	Image       string  `json:"image"`
	State       string  `json:"state"`
	Duration    float64 `json:"duration"`
	CurrentTime float64 `json:"currentTime"`
}

type ccDevice struct {
	Name                   string   `json:"name"`
	Uuid                   string   `json:"uuid"`
	IP                     net.IP   `json:"ip"`
	App                    *ccApp   `json:"app,omitempty"`
	Status                 string   `json:"status"`
	Volume                 int      `json:"volume"`
	Muted                  bool     `json:"muted"`
	Media                  *ccMedia `json:"media,omitempty"`
	Idle                   bool     `json:"idle"`
	Active                 bool     `json:"active"`
	mediaHandler           *handlers.Media
	mediaConnectionHandler *handlers.Connection
	mediaRecieverHandler   *handlers.Receiver
	stateHash              uint32
	pushDebounce           func(f func())
}

func newCcDevice(name string, uuid string, ip net.IP) *ccDevice {
	d := ccDevice{
		Name:                   name,
		Uuid:                   uuid,
		IP:                     ip,
		mediaHandler:           &handlers.Media{},
		mediaConnectionHandler: &handlers.Connection{},
		pushDebounce:           debounce.New(2 * time.Second),
	}
	return &d
}

func newCcApp(name string, id string) *ccApp {
	a := ccApp{
		Name: name,
		ID:   id,
	}
	return &a
}

func newCcMedia(title string, subtitle string, image string, state string, duration float64, currentTime float64) *ccMedia {
	m := ccMedia{
		Title:       title,
		Subtitle:    subtitle,
		Image:       image,
		State:       state,
		Duration:    duration,
		CurrentTime: currentTime,
	}
	return &m
}

func (d *ccDevice) hash() uint32 {
	bytes, err := json.Marshal(d)
	if err != nil {
		fmt.Println("Can't serislize", d)
	}
	h := fnv.New32a()
	h.Write(bytes)
	return h.Sum32()
}

func (d *ccDevice) pushDeviceUpdate() {
	d.pushDebounce(func() {
		if callbackUrls[d.Uuid] != nil {
			for _, url := range callbackUrls[d.Uuid] {
				fmt.Println("Send device Update: " + d.Uuid + " | " + url)
				go d.postUpdateToCallback(url)
			}
		}
	})
}

func (d *ccDevice) postUpdateToCallback(url string) {
	success, err := sendCallbackData(url, callbackAction{Action: "deviceUpdate", Data: d}, d.IP)

	if !success {
		fmt.Println("failed to send deviceUpdate callback to: " + url)
		fmt.Println(err.Error())
	}
}
