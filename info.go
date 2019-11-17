package main

import (
	"fmt"

	"github.com/lugu/qiloop/bus"
)

type info struct {
	service string
	method  string
}

func newInfo(sess bus.Session, w *widgets, service, method string) (*info, error) {
	w.serviceInfo.Reset()
	w.serviceInfo.Write(fmt.Sprintf("Service: %s\n", service))
	w.serviceInfo.Write(fmt.Sprintf("Method: %s", method))
	return &info{
		service: service,
		method:  method,
	}, nil
}
