package main

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/ini.v1"
)

var callbackUrls map[string][]string = make(map[string][]string)

func loadState() {
	inidata, err := ini.Load("state.ini")
	if err != nil {
		fmt.Printf("State ini File cant be found, creating now...")
		inidata = ini.Empty()
		_ = inidata.SaveTo("state.ini")
		return
	}

	section := inidata.Section("callbacks")

	if section == nil {
		return
	}

	for _, did := range section.Keys() {
		callbackUrls[did.Name()] = strings.Split(did.String(), ";")
	}
}

func addCallback(device *ccDevice, url string) bool {
	if callbackUrls[device.Uuid] != nil {
		array := callbackUrls[device.Uuid]
		i := slices.IndexFunc(array, func(u string) bool { return u == url })

		if i > -1 {
			return false
		}
	}

	if callbackUrls[device.Uuid] != nil {
		callbackUrls[device.Uuid] = append(callbackUrls[device.Uuid], url)
	} else {
		callbackUrls[device.Uuid] = []string{url}
	}
	go updateCallbackIni(device, callbackUrls[device.Uuid])

	return true
}

func updateCallbackIni(device *ccDevice, data []string) {
	inidata, err := ini.Load("state.ini")
	if err != nil {
		fmt.Printf("Fail to read state ini file: %v", err)
		inidata = ini.Empty()
	}

	section := inidata.Section("callbacks")

	if section == nil {
		section, _ = inidata.NewSection("callbacks")
	}

	d := strings.Join(data, ";")

	key := section.Key(device.Uuid)

	if key == nil {
		key, _ = section.NewKey(device.Uuid, d)
	} else {
		key.SetValue(d)
	}

	_ = inidata.SaveTo("state.ini")
}

func removeCallback(device *ccDevice, url string) bool {
	if callbackUrls[device.Uuid] == nil {
		return false
	}

	array := callbackUrls[device.Uuid]

	i := slices.IndexFunc(array, func(u string) bool { return u == url })

	if i < 0 {
		return false
	}

	callbackUrls[device.Uuid] = append(array[:i], array[i+1:]...)

	go updateCallbackIni(device, array)
	return true
}
