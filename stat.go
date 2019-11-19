package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lugu/qiloop/bus"
	sd "github.com/lugu/qiloop/bus/services"
	"github.com/mum4k/termdash/container"
)

type entry struct {
	count  bus.MethodStatistics
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

func getObject(sess bus.Session, info sd.ServiceInfo) (bus.ObjectProxy, error) {
	proxy, err := sess.Proxy(info.Name, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to connect service (%s): %s", info.Name, err)
	}
	return bus.MakeObject(proxy), nil
}

func ignoreAction(id uint32) bool {
	if id < 0x50 || id > 0x53 {
		return false
	}
	return true
}

// periodic executes the provided closure periodically every interval.
// Exits when the context expires.
func periodic(ctx context.Context, interval time.Duration, fn func()) {
}

type highlight struct {
	services      map[string]bus.ObjectProxy
	actions       map[string]string
	servicesMutex sync.Mutex
}

func newHighlighter(ctx context.Context, cancel context.CancelFunc, c *container.Container, w *widgets) (*highlight, error) {

	h := &highlight{
		services: map[string]bus.ObjectProxy{},
		actions:  map[string]string{},
	}

	err := h.initServices(ctx, sess, cancel)
	if err != nil {
		return nil, err
	}

	updater, err := h.updater(ctx, sess, cancel)
	if err != nil {
		return nil, err
	}

	onSelect := func(index int, line string) error {
		if index == 0 {
			setLayout(c, w, layoutTop)
			if w.collector != nil {
				w.collector.cancel()
				w.collector = nil
			}
			if w.logger != nil {
				w.logger.cancel()
				w.logger = nil
			}
			return nil
		}
		setLayout(c, w, layoutTopTraceLogs)

		labels := strings.SplitN(line, " | ", 5)
		if len(labels) != 5 {
			return fmt.Errorf("invalid line: %s", line)
		}
		desc := strings.SplitN(labels[4], ".", 2)
		if len(desc) != 2 {
			return fmt.Errorf("invalid service.action: %s", labels[4])
		}
		err := selectMethod(c, w, desc[0], desc[1])
		if err != nil {
			return err
		}
		return nil
	}

	w.topList.Configure([]string{}, onSelect)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lines, err := updater()
				if err != nil {
					mainErr = err
					cancel()
				}
				w.topList.Configure(lines, onSelect)
			case <-ctx.Done():
				return
			}
		}
	}()
	return h, nil
}

func (h *highlight) updateService(serviceName string, info sd.ServiceInfo) error {
	obj, err := getObject(sess, info)
	if err != nil {
		return err
	}

	meta, err := obj.MetaObject(obj.ObjectID())
	if err != nil {
		return err
	}
	h.servicesMutex.Lock()
	defer h.servicesMutex.Unlock()
	h.services[serviceName] = obj
	for id, method := range meta.Methods {
		if ignoreAction(id) {
			continue
		}
		actionID := fmt.Sprintf("%s.%d", serviceName, id)
		actionName := fmt.Sprintf("%s.%s", serviceName, method.Name)
		h.actions[actionID] = actionName
	}
	return nil
}

// returns a function which update the top statistics
func (h *highlight) initServices(ctx context.Context, sess bus.Session, cancel context.CancelFunc) error {
	proxies := sd.Services(sess)

	onDisconnect := func(err error) {
		mainErr = fmt.Errorf("Service directory disconnection: %s", err)
		cancel()
	}

	directory, err := proxies.ServiceDirectory(onDisconnect)
	if err != nil {
		return err
	}

	serviceList, err := directory.Services()
	if err != nil {
		return err
	}

	for _, info := range serviceList {
		err = h.updateService(info.Name, info)
		if err != nil {
			return err
		}
	}

	cancelAdded, added, err := directory.SubscribeServiceAdded()
	if err != nil {
		return err
	}
	cancelRemoved, removed, err := directory.SubscribeServiceRemoved()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				cancelAdded()
				cancelRemoved()
				return
			case srv := <-added:
				info, err := directory.Service(srv.Name)
				if err != nil {
					log.Print(err)
					continue
				}
				err = h.updateService(srv.Name, info)
				if err != nil {
					log.Print(err)
					continue
				}
			case srv := <-removed:
				h.servicesMutex.Lock()
				if _, ok := h.services[srv.Name]; ok {
					delete(h.services, srv.Name)
				}
				h.servicesMutex.Unlock()
			}
		}
	}()

	// enable stats
	for _, obj := range h.services {
		obj.EnableStats(true)
		obj.ClearStats()

	}
	go func(ctx context.Context) {
		<-ctx.Done()

		for _, obj := range h.services {
			obj.EnableStats(false)
		}
	}(ctx)
	return nil
}

func (h *highlight) updater(ctx context.Context, sess bus.Session, cancel context.CancelFunc) (func() ([]string, error), error) {

	return func() ([]string, error) {
		for {
			counter := map[string]bus.MethodStatistics{}
			h.servicesMutex.Lock()
			for name, obj := range h.services {
				stats, err := obj.Stats()
				if err != nil {
					continue
				}
				for action, stat := range stats {
					if ignoreAction(action) {
						continue
					}
					entry := fmt.Sprintf("%s.%d", name, action)
					action, ok := h.actions[entry]
					if !ok {
						continue
					}
					counter[action] = stat
				}
			}
			h.servicesMutex.Unlock()
			topC := make([]entry, 0)
			for action, count := range counter {
				if count.Count == 0 {
					continue
				}
				topC = append(topC, entry{
					action: action,
					count:  count,
				})
			}
			sort.Sort(gallery(topC))
			lines := make([]string, len(topC)+1)
			lines[0] = " count | min (us) | max (us) | avg (us) | Service.Method"
			for i, entry := range topC {
				lines[i+1] = fmt.Sprintf(" %5d | %8.0f | %8.0f | %8.0f | %s",
					entry.count.Count,
					entry.count.Wall.MinValue*1000000.0,
					entry.count.Wall.MaxValue*1000000.0,
					entry.count.Wall.CumulatedValue*1000000.0/float32(entry.count.Count),
					entry.action)
			}
			return lines, nil
		}
	}, nil
}
