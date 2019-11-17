package main

import (
	"fmt"
	"math"
	"time"

	"github.com/lugu/qiloop/bus"
	"github.com/lugu/qiloop/bus/net"
	"github.com/lugu/qiloop/type/value"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/widgets/linechart"
)

type callEvent struct {
	timestamp    time.Time
	duration     time.Duration
	callSize     int
	replySize    int
	userUsTime   int64
	systemUsTime int64
	responseType uint8
}

func newCallEvent(call, response bus.EventTrace) callEvent {

	if uint8(call.Kind) != net.Call {
		panic(call)
	}

	since := time.Unix(call.Timestamp.Tv_sec, call.Timestamp.Tv_usec*1000)
	until := time.Unix(response.Timestamp.Tv_sec, response.Timestamp.Tv_usec*1000)

	return callEvent{
		timestamp:    since,
		duration:     until.Sub(since),
		callSize:     net.HeaderSize + len(value.Bytes(call.Arguments)),
		replySize:    net.HeaderSize + len(value.Bytes(response.Arguments)),
		userUsTime:   response.UserUsTime,
		systemUsTime: response.SystemUsTime,
		responseType: uint8(response.Kind),
	}
}

type collector struct {
	service string
	action  string
	slot    uint32

	callData         []float64
	replyData        []float64
	latencyData      []float64
	latencyErrorData []float64
	sysTimeData      []float64
	usrTimeData      []float64

	cancel func()

	records []callEvent
	pending map[uint32]bus.EventTrace
}

func newCollector(sess bus.Session, w *widgets, service, action string) (*collector, error) {
	objectID := uint32(1)
	proxy, err := sess.Proxy(service, objectID)
	if err != nil {
		return nil, fmt.Errorf("trace %s: %s", service, err)
	}
	obj := bus.MakeObject(proxy)

	meta, err := obj.MetaObject(objectID)
	if err != nil {
		return nil, fmt.Errorf("%s: MetaObject: %s.", service, err)
	}

	slot, err := meta.MethodID(action)
	if err != nil {
		return nil, fmt.Errorf("method not found: %s.", action)
	}

	obj.EnableTrace(true)
	if err != nil {
		return nil, fmt.Errorf("Failed to start traces: %s", err)
	}

	cancel, events, err := obj.SubscribeTraceObject()
	if err != nil {
		return nil, fmt.Errorf("Failed to stop stats: %s.", err)
	}

	c := &collector{
		service: service,
		action:  action,
		slot:    slot,

		cancel: cancel,

		pending: map[uint32]bus.EventTrace{},
		records: []callEvent{},

		callData:         []float64{},
		replyData:        []float64{},
		latencyData:      []float64{},
		latencyErrorData: []float64{},
		sysTimeData:      []float64{},
		usrTimeData:      []float64{},
	}

	go func(events chan bus.EventTrace) {
		for {
			e, ok := <-events
			if !ok {
				obj.EnableTrace(false)
				return
			}
			c.refreshData(e)
			c.updateUI(w)
		}
	}(events)

	return c, nil
}

func (c *collector) updateData(evt callEvent) {
	c.records = append(c.records, evt)

	if evt.responseType == net.Reply {
		c.latencyData = append(c.latencyData, float64(evt.duration.Microseconds()))
		c.latencyErrorData = append(c.latencyErrorData, math.NaN())
	} else {
		c.latencyData = append(c.latencyData, math.NaN())
		c.latencyErrorData = append(c.latencyErrorData, float64(evt.duration.Microseconds()))
	}
	c.sysTimeData = append(c.sysTimeData, float64(evt.systemUsTime))
	c.usrTimeData = append(c.usrTimeData, float64(evt.userUsTime))
	c.callData = append(c.callData, float64(evt.callSize))
	c.replyData = append(c.replyData, float64(evt.replySize))
}

func (c *collector) refreshData(e bus.EventTrace) {
	if e.SlotId != c.slot {
		return
	}

	e0, ok := c.pending[e.Id]
	if !ok {
		c.pending[e.Id] = e
		return
	}
	delete(c.pending, e.Id)

	call, response := e0, e
	if e.Kind == int32(net.Call) {
		call, response = e, e0
	}
	c.updateData(newCallEvent(call, response))
}

func noMoreThan(max int, data []float64) []float64 {
	start := 0
	if max == 0 {
		return []float64{}
	}
	if max > 0 && max < len(data) {
		start = len(data) - max
	}
	return data[start:]
}

func (c *collector) updateUI(w *widgets) {
	w.latencyPlot.Series("response time",
		noMoreThan(w.latencyPlot.ValueCapacity(), c.latencyData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorYellow)),
	)
	w.latencyPlot.Series("error response time",
		noMoreThan(w.latencyPlot.ValueCapacity(), c.latencyErrorData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorRed)),
	)
	w.timePlot.Series("user time",
		noMoreThan(w.timePlot.ValueCapacity(), c.usrTimeData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	w.timePlot.Series("system time",
		noMoreThan(w.timePlot.ValueCapacity(), c.sysTimeData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorYellow)),
	)
	w.sizePlot.Series("call size",
		noMoreThan(w.sizePlot.ValueCapacity(), c.callData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	w.sizePlot.Series("reply size",
		noMoreThan(w.sizePlot.ValueCapacity(), c.replyData),
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorYellow)),
	)
}
