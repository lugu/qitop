package main

import (
	"fmt"

	"github.com/lugu/qiloop/bus"
	qilog "github.com/lugu/qiloop/bus/logger"
	"github.com/lugu/qiloop/bus/services"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/widgets/text"
)

func label(l qilog.LogLevel) (cell.Color, string) {
	switch l {
	case qilog.LogLevelFatal:
		return cell.ColorRed, "[F]"
	case qilog.LogLevelError:
		return cell.ColorRed, "[E]"
	case qilog.LogLevelWarning:
		return cell.ColorYellow, "[W]"
	case qilog.LogLevelInfo:
		return cell.ColorMagenta, "[I]"
	case qilog.LogLevelVerbose:
		return cell.ColorDefault, "[V]"
	case qilog.LogLevelDebug:
		return cell.ColorRGB6(3, 3, 3), "[D]"
	default:
		return cell.ColorRed, "[?]"
	}
}

type logger struct {
	cancel func()
}

func newLogger(sess bus.Session, w *widgets, service, method string) (*logger, error) {
	w.logScroll.Reset()

	directory, err := services.Services(sess).ServiceDirectory(nil)
	if err != nil {
		return nil, err
	}
	info, err := directory.Service(service)
	if err != nil {
		return nil, fmt.Errorf("service not found (%s): %s", service, err)
	}
	location := fmt.Sprintf("%s:%d", info.MachineId, info.ProcessId)

	srv := qilog.Services(sess)
	logManager, err := srv.LogManager(nil)
	if err != nil {
		return nil, fmt.Errorf("access LogManager service: %s", err)
	}
	logListener, err := logManager.CreateListener()
	if err != nil {
		return nil, fmt.Errorf("create listener: %s", err)
	}

	err = logListener.ClearFilters()
	if err != nil {
		return nil, fmt.Errorf("clear filters: %s", err)
	}
	cancel, logs, err := logListener.SubscribeOnLogMessages()
	if err != nil {
		return nil, fmt.Errorf("subscribe logs: %s", err)
	}

	err = logListener.SetLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("set verbosity: %s", err)
	}

	go func() {
		defer logListener.Terminate(logListener.ObjectID())
		for {
			msgs, ok := <-logs
			for _, m := range msgs {
				if !ok {
					w.logScroll.Reset()
					return
				}
				if m.Level == qilog.LogLevelNone {
					continue
				}
				if m.Location != location {
					continue
				}
				color, info := label(m.Level)
				message := fmt.Sprintf("%s %s\n", info, m.Message)
				opt := text.WriteCellOpts(cell.FgColor(color))
				w.logScroll.Write(message, opt)
			}
		}
	}()
	return &logger{
		cancel: cancel,
	}, nil
}
