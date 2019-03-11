package main

import (
	"flag"
	"fmt"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/lugu/qiloop/bus"
	"github.com/lugu/qiloop/bus/client/object"
	"github.com/lugu/qiloop/bus/client/services"
	"github.com/lugu/qiloop/bus/session"
	"log"
	"os"
	"os/signal"
	"sort"
	"time"
)

type collector struct {
	services map[string]object.ObjectProxy
	counter  map[string]object.MethodStatistics
	actions  map[string]string
}

func ignoreAction(id uint32) bool {
	if id < 0x50 || id > 0x53 {
		return false
	}
	return true
}

func NewCollector(services map[string]object.ObjectProxy) *collector {

	c := &collector{
		services: services,
		counter:  make(map[string]object.MethodStatistics),
		actions:  make(map[string]string),
	}

	for servicename, obj := range services {
		meta, err := obj.MetaObject(obj.ObjectID())
		if err != nil {
			log.Fatal(err)
		}
		for id, method := range meta.Methods {
			if ignoreAction(id) {
				continue
			}
			actionID := fmt.Sprintf("%s.%d", servicename, id)
			actionName := fmt.Sprintf("%s.%s", servicename, method.Name)
			c.actions[actionID] = actionName
			c.counter[actionName] = object.MethodStatistics{}
		}
	}
	return c
}

func getObject(sess bus.Session, info services.ServiceInfo) object.ObjectProxy {
	proxy, err := sess.Proxy(info.Name, 1)
	if err != nil {
		log.Fatalf("failed to connect service (%s): %s", info.Name, err)
	}
	return object.MakeObject(proxy)
}

func (c *collector) updateStat(name string, statistics map[uint32]object.MethodStatistics) error {
	for action, stat := range statistics {
		if ignoreAction(action) {
			continue
		}
		entry := fmt.Sprintf("%s.%d", name, action)
		action := c.actions[entry]
		c.counter[action] = stat
	}
	return nil
}

type entry struct {
	count  object.MethodStatistics
	action string
}
type gallery []entry

func (e gallery) Len() int      { return len(e) }
func (e gallery) Swap(i, j int) { e[i], e[j] = e[j], e[i] }
func (e gallery) Less(i, j int) bool {
	if e[i].count.Count == e[j].count.Count {
		return e[i].count.Wall.CumulatedValue >
			e[j].count.Wall.CumulatedValue
	}
	return e[i].count.Count > e[j].count.Count
}

func (c *collector) top() []string {
	counter := make([]entry, 0)
	for action, count := range c.counter {
		counter = append(counter, entry{
			action: action,
			count:  count,
		})
	}
	sort.Sort(gallery(counter))
	lines := make([]string, len(c.counter))
	for i, entry := range counter {
		lines[i] = fmt.Sprintf("[%04d] %s: %d", i+1,
			entry.action, entry.count.Count)
	}
	return lines
}

func (c *collector) updateStream() chan []string {
	out := make(chan []string)

	go func() {
		ticker := time.Tick(1000 * time.Millisecond)
		for {
			<-ticker
			list, err := c.update()
			if err != nil {
				fmt.Print(err)
				close(out)
			}
			out <- list
		}
	}()
	return out
}

func (c *collector) update() ([]string, error) {
	for name, obj := range c.services {
		stats, err := obj.Stats()
		if err != nil {
			return nil, err
		}
		if err := c.updateStat(name, stats); err != nil {
			return nil, err
		}

	}
	return c.top(), nil
}

func loopBatch(sess bus.Session, services map[string]object.ObjectProxy) {
	c := NewCollector(services)
	updates := c.updateStream()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		select {
		case s := <-interrupt:
			log.Printf("%v: quitting.", s)
			return
		case lines, ok := <-updates:
			if !ok {
				log.Printf("Remote error")
				return
			}
			for _, line := range lines {
				fmt.Printf("%s\n", line)
			}
		}
	}
}

func loopTermUI(sess bus.Session, services map[string]object.ObjectProxy) {

	if err := ui.Init(); err != nil {
		log.Print(err)
		return
	}
	defer ui.Close()

	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	c := NewCollector(services)

	list := widgets.NewList()
	list.Title = "Most used methods"
	list.TextStyle = ui.NewStyle(ui.ColorYellow)
	list.WrapText = false
	list.SetRect(0, 0, 25, 8)
	list.Rows = c.top()

	grid.Set(ui.NewRow(1.0, ui.NewCol(1.0, list)))
	ui.Render(grid)

	uiEvents := ui.PollEvents()
	updates := c.updateStream()

	for {
		select {
		case lines, ok := <-updates:
			if !ok {
				log.Printf("Remote error")
				return
			}
			list.Rows = lines
			grid.Set(ui.NewRow(1.0, ui.NewCol(1.0, list)))
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "j", "<Down>":
				list.ScrollDown()
			case "k", "<Up>":
				list.ScrollUp()
			case "<C-d>":
				list.ScrollHalfPageDown()
			case "<C-u>":
				list.ScrollHalfPageUp()
			case "<C-f>":
				list.ScrollPageDown()
			case "<C-b>":
				list.ScrollPageUp()
			case "<Home>":
				list.ScrollTop()
			case "G", "<End>":
				list.ScrollBottom()
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
			}
		}
		ui.Render(grid)
	}
}

func main() {
	var serverURL = flag.String("qi-url", "tcp://127.0.0.1:9559", "server URL")
	var bactchMode = flag.Bool("b", false, "batch mode (default disable)")
	flag.Parse()

	sess, err := session.NewSession(*serverURL)
	if err != nil {
		log.Fatal(err)
	}

	proxies := services.Services(sess)

	directory, err := proxies.ServiceDirectory()
	if err != nil {
		log.Fatal(err)
	}

	serviceList, err := directory.Services()
	if err != nil {
		log.Fatal(err)
	}

	services := make(map[string]object.ObjectProxy)

	for _, info := range serviceList {
		services[info.Name] = getObject(sess, info)
	}

	// enable stats
	for _, obj := range services {
		obj.EnableStats(true)
		obj.ClearStats()

	}
	defer func() {
		for _, obj := range services {
			obj.EnableStats(false)
		}
	}()

	// print stats
	if *bactchMode {
		loopBatch(sess, services)
	} else {
		loopTermUI(sess, services)
	}
}
