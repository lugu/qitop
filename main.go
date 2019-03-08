package main

import (
	"fmt"
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
	counter  map[string]uint32
}

func NewCollector(services map[string]object.ObjectProxy) *collector {
	return &collector{
		services: services,
		counter:  make(map[string]uint32),
	}
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
		c.counter[entry] += stat.Count
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
	for _, entry := range counter {
		fmt.Printf("%s: %d\n", entry.action, entry.count)
	}
}

func (c *collector) update() error {
	for name, obj := range c.services {
		stats, err := obj.Stats()
		if err != nil {
			return err
		}
		if err := c.updateStat(name, stats); err != nil {
			return err
		}

	}
	return nil
}

func loop(sess bus.Session, services map[string]object.ObjectProxy) {

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ticker := time.Tick(1000 * time.Millisecond)

	collector := NewCollector(services)
	for {
		select {
		case s := <-c:
			log.Printf("Got signal: %v", s)
			return
		case _ = <-ticker:

			if err := collector.update(); err != nil {
				log.Printf("Error: %v", err)
			}
			collector.print()
		}
	}
}

func main() {
	sess, err := session.NewSession("tcp://localhost:9559")
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

	// enable stats
	for _, obj := range services {
		obj.EnableStats(true)

	}

	// print stats
	loop(sess, services)

	// stop stats
	for _, obj := range services {
		obj.EnableStats(false)

	}
	log.Printf("Terminated.")
}
