package main

type callbackAction struct {
	Action string    `json:"action"`
	Data   *ccDevice `json:"data,omitempty"`
}
