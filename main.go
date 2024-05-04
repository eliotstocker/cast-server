package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stampzilla/gocast/discovery"
	"github.com/stampzilla/gocast/events"
	"golang.org/x/exp/maps"
)

var discoveredDevices map[string]*ccDevice = make(map[string]*ccDevice)
var mutex = &sync.RWMutex{}

func main() {
	logrus.SetLevel(logrus.ErrorLevel)
	loadState()
	discovery := discovery.NewService()

	go discoveryListner(discovery)

	// Start a periodic discovery
	fmt.Println("Start discovery")
	discovery.Start(context.Background(), time.Second*5)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleList)
	mux.HandleFunc("/{uuid}", handleGet)
	mux.HandleFunc("/{uuid}/pause", handlePause)
	mux.HandleFunc("/{uuid}/play", handlePlay)
	mux.HandleFunc("/{uuid}/stop", handleStop)
	mux.HandleFunc("/{uuid}/volume", handleVolume)
	mux.HandleFunc("/{uuid}/subscribe", handleRegister)
	mux.HandleFunc("/{uuid}/unsubscribe", handleDeregister)

	fmt.Println("Start HTTP Server")
	err := http.ListenAndServe(":3333", mux)

	if err != nil {
		fmt.Println("Cant start http server!")
		os.Exit(1)
	}
}

func handleList(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	array := maps.Values(discoveredDevices)
	mutex.RUnlock()

	sort.Slice(array, func(i, j int) bool { return array[i].Uuid < array[j].Uuid })

	bytes, err := json.Marshal(array)

	if err != nil {
		fmt.Println("Can't serislize", array)
		handleError(w, "list", "cant serialise current device list", 500)
		return
	}

	io.WriteString(w, string(bytes))
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	handled, d := handleControl(w, r, "Get")

	if !handled {
		bytes, err := json.Marshal(d)
		if err != nil {
			fmt.Println("Can't serislize", d)
		}

		w.Write(bytes)
	}
}

func handleControl(w http.ResponseWriter, r *http.Request, operation string) (bool, *ccDevice) {
	uuid := r.PathValue("uuid")

	mutex.Lock()
	d := discoveredDevices[uuid]
	mutex.Unlock()

	if d == nil {
		handleError(w, operation, "UUID not found", 404)
		return true, nil
	}

	return false, d
}

func handleSuccess(w http.ResponseWriter, operation string) {
	out := newControl("success", operation)

	bytes, err := json.Marshal(out)
	if err != nil {
		fmt.Println("Can't serislize", out)
	}
	w.Write(bytes)
}

func handleError(w http.ResponseWriter, operation string, errorString string, code int) {
	out := newControl("error", operation)
	out.Error = errorString

	bytes, err := json.Marshal(out)
	if err != nil {
		fmt.Println("Can't serislize", out)
	}

	w.WriteHeader(code)
	w.Write(bytes)
}

func handlePause(w http.ResponseWriter, r *http.Request) {
	handled, d := handleControl(w, r, "pause")

	if !handled {
		d.mediaHandler.Pause()
		handleSuccess(w, "pause")
	}
}

func handlePlay(w http.ResponseWriter, r *http.Request) {
	handled, d := handleControl(w, r, "play")

	if !handled {
		d.mediaHandler.Play()
		handleSuccess(w, "play")
	}
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	handled, d := handleControl(w, r, "stop")

	if !handled {
		d.mediaHandler.Stop()
		handleSuccess(w, "stop")
	}
}

func handleVolume(w http.ResponseWriter, r *http.Request) {
	handled, d := handleControl(w, r, "volume")

	if !handled {
		value := r.URL.Query().Get("value")

		if value == "" {
			handleError(w, "volume", "missing value parameter", 400)
			return
		}

		volume, err := strconv.ParseInt(value, 10, 16)

		if err != nil {
			handleError(w, "volume", "value parameter invalid", 400)
			return
		}

		d.mediaRecieverHandler.SetVolume(float64(volume) / 100)
		handleSuccess(w, "volume")
	}
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		handleError(w, "register", "callback registration must be a POST request", 405)
		return
	}

	handled, d := handleControl(w, r, "subscribe")

	if !handled {
		value := r.URL.Query().Get("url")

		if value == "" {
			handleError(w, "register", "missing url parameter", 400)
			return
		}

		//Ping the url to make sure its valid
		success, _ := sendCallbackData(value, map[string]string{
			"action": "ping",
		}, nil)

		if success {
			added := addCallback(d, value)

			if added {
				handleSuccess(w, "register")
				return
			}
			handleError(w, "register", "could not add callback url as it is already present", 400)
			return
		}

		handleError(w, "register", "could not post \"ping\" action to passed URL", 404)
	}
}

func handleDeregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		handleError(w, "register", "callback registration must be a POST request", 405)
		return
	}

	handled, d := handleControl(w, r, "subscribe")

	if !handled {
		value := r.URL.Query().Get("url")

		if value == "" {
			handleError(w, "register", "missing url parameter", 400)
			return
		}

		found := removeCallback(d, value)

		if !found {
			handleError(w, "deregister", "url: \""+value+"\" not registered", 404)
			return
		}

		handleSuccess(w, "deregister")
	}
}

func sendCallbackData(callbackUrl string, data any, from net.IP) (bool, error) {
	postBody, _ := json.Marshal(data)

	responseBody := bytes.NewBuffer(postBody)

	client := &http.Client{}

	req, err := http.NewRequest("POST", callbackUrl, responseBody)
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")

	if from != nil {
		req.Header.Set("X-Forwarded-For", from.String())
	}

	_, err = client.Do(req)

	if err != nil {
		return false, err
	}

	return true, nil
}

func discoveryListner(discovery *discovery.Service) {
	for device := range discovery.Found() {
		device.OnEvent(func(event events.Event) {

			mutex.Lock()
			d := discoveredDevices[device.Uuid()]
			switch data := event.(type) {
			case events.Connected:
				fmt.Println("Connected: " + device.Name())
				d.Status = "IDLE"
			case events.Disconnected:
				fmt.Println("Disconnected: " + device.Name())
				delete(discoveredDevices, device.Uuid())
			case events.AppStarted:
				d.App = newCcApp(data.DisplayName, data.AppID)

				if data.HasNamespace("urn:x-cast:com.google.cast.media") {
					device.Subscribe("urn:x-cast:com.google.cast.tp.connection", data.TransportId, d.mediaConnectionHandler)
					device.Subscribe("urn:x-cast:com.google.cast.media", data.TransportId, d.mediaHandler)
				}
			case events.AppStopped:
				d.App = nil
				d.Media = nil
				d.Status = "IDLE"
				for _, v := range data.Namespaces {
					device.UnsubscribeByUrnAndDestinationId(v.Name, data.TransportId)
				}
			case events.Media:
				d.Status = data.PlayerState
				d.Volume = int(math.Round(data.Volume.Level * 100))
				d.Muted = data.Volume.Muted
				if data.Media != nil {
					var imgUrl string
					if len(data.Media.MetaData.Images) > 0 {
						imgUrl = data.Media.MetaData.Images[0].Url
					}

					d.Media = newCcMedia(
						data.Media.MetaData.Title,
						data.Media.MetaData.SubTitle,
						imgUrl,
						data.PlayerState,
						data.Media.Duration,
						data.CurrentTime,
					)
				}
			case events.ReceiverStatus:
				d.Volume = int(math.Round(data.Status.Volume.Level * 100))
				d.Muted = data.Status.Volume.Muted
				d.Idle = data.Status.IsStandBy
				d.Active = data.Status.IsActiveInput
			default:
				fmt.Printf("unexpected event %T: %#v\n", data, data)
			}

			newHash := d.hash()
			if newHash != d.stateHash {
				d.stateHash = newHash
				d.pushDeviceUpdate()
			}
			mutex.Unlock()
		})

		newDevice := newCcDevice(device.Name(), device.Uuid(), device.Ip())
		newDevice.Status = "CONNECTING"
		newDevice.stateHash = newDevice.hash()
		newDevice.mediaRecieverHandler = device.ReceiverHandler

		mutex.Lock()
		discoveredDevices[device.Uuid()] = newDevice
		mutex.Unlock()

		fmt.Println("found: " + device.Uuid())
		device.Connect(context.Background())
	}
}
