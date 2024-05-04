package main

type control struct {
	Status    string `json:"status"`
	Operation string `json:"operation"`
	Error     string `json:"error,omitempty"`
}

func newControl(status string, operation string) control {
	return control{Status: status, Operation: operation}
}
