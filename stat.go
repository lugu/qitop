package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lugu/qiloop/bus"
	"github.com/lugu/qiloop/bus/services"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/text"
)

// periodic executes the provided closure periodically every interval.
// Exits when the context expires.
func periodic(ctx context.Context, interval time.Duration, fn func()) {
}

type highlight struct {
	index   int
	lines   []string
	counter []entry
}

func newHighlighter(ctx context.Context, cancel context.CancelFunc, w *widgets) (*highlight, error) {
	updater, err := statUpdater(ctx, sess, cancel)
	if err != nil {
		return nil, err
	}
	h := &highlight{
		index: 0,
		lines: []string{},
	}

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
				h.lines = lines
				h.updateTopList(w)
			case <-ctx.Done():
				return
			}
		}
	}()
	return h, nil
}

func (h *highlight) key(c *container.Container, w *widgets, k *terminalapi.Keyboard) error {
	switch k.Key {
	case 'k', keyboard.KeyArrowUp:
		if h.index > 0 {
			h.index--
		}
		h.updateTopList(w)
	case 'j', keyboard.KeyArrowDown:
		if h.index < len(h.lines)-1 {
			h.index++
		}
		h.updateTopList(w)
	case keyboard.KeyEnter:
		if h.index == 0 {
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

		line := h.lines[h.index]
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
	}
	return nil
}

func (h *highlight) updateTopList(w *widgets) {
	w.topList.Reset()
	for i, line := range h.lines {
		l := fmt.Sprintf("%s\n", line)
		if i == h.index {
			opt := text.WriteCellOpts(cell.FgColor(cell.ColorYellow))
			w.topList.Write(l, opt)
		} else {
			w.topList.Write(l)
		}
	}
}

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
		return nil, err
	}

	serviceList, err := directory.Services()
	if err != nil {
		return nil, err
	}

	services := map[string]bus.ObjectProxy{}
	counter := map[string]bus.MethodStatistics{}
	actions := map[string]string{}

	for _, info := range serviceList {
		services[info.Name], err = getObject(sess, info)
		if err != nil {
			return nil, err
		}
	}

	for servicename, obj := range services {
		meta, err := obj.MetaObject(obj.ObjectID())
		if err != nil {
			return nil, err
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
