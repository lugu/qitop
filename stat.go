package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/lugu/qiloop/bus"
	"github.com/lugu/qiloop/bus/services"
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

func getObject(sess bus.Session, info services.ServiceInfo) (bus.ObjectProxy, error) {
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

// returns a function which update the top statistics
func statUpdater(ctx context.Context, sess bus.Session, cancel context.CancelFunc) (func() ([]string, error), error) {
	proxies := services.Services(sess)

	onDisconnect := func(err error) {
		mainErr = fmt.Errorf("Service directory disconnection: %s", err)
		cancel()
	}
	directory, err := proxies.ServiceDirectory(onDisconnect)
	if err != nil {
		mainErr = err
		cancel()
	}

	serviceList, err := directory.Services()
	if err != nil {
		mainErr = err
		cancel()
	}

	services := map[string]bus.ObjectProxy{}
	counter := map[string]bus.MethodStatistics{}
	actions := map[string]string{}

	for _, info := range serviceList {
		services[info.Name], err = getObject(sess, info)
		if err != nil {
			mainErr = err
			cancel()
		}
	}

	for servicename, obj := range services {
		meta, err := obj.MetaObject(obj.ObjectID())
		if err != nil {
			mainErr = err
			cancel()
		}
		for id, method := range meta.Methods {
			if ignoreAction(id) {
				continue
			}
			actionID := fmt.Sprintf("%s.%d", servicename, id)
			actionName := fmt.Sprintf("%s.%s", servicename, method.Name)
			actions[actionID] = actionName
			counter[actionName] = bus.MethodStatistics{}
		}
	}

	// enable stats
	for _, obj := range services {
		obj.EnableStats(true)
		obj.ClearStats()

	}
	go func(ctx context.Context) {
		<-ctx.Done()

		for _, obj := range services {
			obj.EnableStats(false)
		}
	}(ctx)
	return func() ([]string, error) {
		for {
			for name, obj := range services {
				stats, err := obj.Stats()
				if err != nil {
					continue
				}
				for action, stat := range stats {
					if ignoreAction(action) {
						continue
					}
					entry := fmt.Sprintf("%s.%d", name, action)
					action := actions[entry]
					counter[action] = stat
				}
			}
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
