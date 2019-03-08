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
	"sort"
	"sync"
	"time"
)

type collector struct {
	services    map[string]object.ObjectProxy
	counter     map[string]uint32
	actions     map[string]string
	uiList      *widgets.List
	uiListMutex sync.Mutex
}

func NewCollector(services map[string]object.ObjectProxy) *collector {

	// enable stats
	for _, obj := range services {
		obj.EnableStats(true)

	}

	c := &collector{
		services: services,
		counter:  make(map[string]uint32),
		actions:  make(map[string]string),
		uiList:   widgets.NewList(),
	}

	c.uiList.Title = "Methods"
	c.uiList.TextStyle = ui.NewStyle(ui.ColorYellow)
	c.uiList.WrapText = false
	c.uiList.SetRect(0, 0, 25, 8)
	c.uiList.Rows = make([]string, 0)

	for servicename, obj := range services {
		meta, err := obj.MetaObject(obj.ObjectID())
		if err != nil {
			panic(err)
		}
		for id, method := range meta.Methods {
			actionID := fmt.Sprintf("%s.%d", servicename, id)
			actionName := fmt.Sprintf("%s.%s", servicename, method.Name)
			c.actions[actionID] = actionName
			c.counter[actionName] = 0
			toPrint := fmt.Sprintf("%s: %d", actionName, 0)
			c.uiList.Rows = append(c.uiList.Rows, toPrint)
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
		entry := fmt.Sprintf("%s.%d", name, action)
		action := c.actions[entry]
		c.counter[action] += stat.Count
	}
	return nil
}

type entry struct {
	count  uint32
	action string
}
type gallery []entry

func (e gallery) Len() int           { return len(e) }
func (e gallery) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e gallery) Less(i, j int) bool { return e[i].count < e[j].count }

func (c *collector) print() {
	var counter gallery = make([]entry, 0)
	for action, count := range c.counter {
		counter = append(counter, entry{
			action: action,
			count:  count,
		})
	}
	sort.Sort(counter)
	/* FIXME
	for _, entry := range counter {
		fmt.Printf("%s: %d\n", entry.action, entry.count)
	}
	*/
}

func (c *collector) update(errors chan error) {
	for name, obj := range c.services {
		stats, err := obj.Stats()
		if err != nil {
			errors <- err
			return
		}
		if err := c.updateStat(name, stats); err != nil {
			errors <- err
			return
		}

	}
	c.print()
}

func loop(sess bus.Session, services map[string]object.ObjectProxy) {

	if err := ui.Init(); err != nil {
		log.Print(err)
		return
	}
	defer ui.Close()

	c := NewCollector(services)
	defer func() {
		for _, obj := range c.services {
			obj.EnableStats(false)
		}
		log.Printf("Terminated.")
	}()

	ui.Render(c.uiList)
	uiEvents := ui.PollEvents()

	errors := make(chan error)

	ticker := time.Tick(1000 * time.Millisecond)

	for {
		select {
		case err := <-errors:
			log.Printf("Error: %s", err)
			return
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "j", "<Down>":
				c.uiList.ScrollDown()
			case "k", "<Up>":
				c.uiList.ScrollUp()
			case "<C-d>":
				c.uiList.ScrollHalfPageDown()
			case "<C-u>":
				c.uiList.ScrollHalfPageUp()
			case "<C-f>":
				c.uiList.ScrollPageDown()
			case "<C-b>":
				c.uiList.ScrollPageUp()
			case "<Home>":
				c.uiList.ScrollTop()
			case "G", "<End>":
				c.uiList.ScrollBottom()
			}
		case _ = <-ticker:
			go c.update(errors)
		}
		ui.Render(c.uiList)
	}
}

func main() {
	var serverURL = flag.String("qi-url", "tcp://127.0.0.1:9559", "server URL")
	flag.Parse()

	sess, err := session.NewSession(*serverURL)
	if err != nil {
		panic(err)
	}

	proxies := services.Services(sess)

	directory, err := proxies.ServiceDirectory()
	if err != nil {
		panic(err)
	}

	serviceList, err := directory.Services()
	if err != nil {
		panic(err)
	}

	services := make(map[string]object.ObjectProxy)

	for _, info := range serviceList {
		services[info.Name] = getObject(sess, info)
	}

	// print stats
	loop(sess, services)
}
