package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lugu/qiloop/app"
	"github.com/lugu/qiloop/bus"
	qilog "github.com/lugu/qiloop/bus/logger"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/container/grid"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"

	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
)

const (
	// rootID is the ID assigned to the root container.
	rootID = "root"

	// redrawInterval is how often termdash redraws the screen.
	redrawInterval = 250 * time.Millisecond
)

// layoutType represents the possible UI layouts
type layoutType int

const (
	// layoutTop: only shows method usage
	layoutTop layoutType = iota
	// layoutTop: shows top method usage and trace
	layoutTopTrace
	// layoutTop: shows method usage, trace and logs
	layoutTopTraceLogs
)

var (
	sess bus.Session

	// log level displayed
	logLevel qilog.LogLevel

	// application error status
	mainErr error = nil
)

// widgets holds the widgets used by this demo.
type widgets struct {
	topList     *text.Text
	logScroll   *text.Text
	latencyPlot *linechart.LineChart
	timePlot    *linechart.LineChart
	sizePlot    *linechart.LineChart

	index   int
	lines   []string
	counter []entry

	collector *collector
	logger    *logger
}

func (w *widgets) refreshTopList(lines []string) {
	w.lines = lines
	w.updateTopList()
}

func (w *widgets) updateTopList() {
	w.topList.Reset()
	for i, line := range w.lines {
		l := fmt.Sprintf("%s\n", line)
		if i == w.index {
			opt := text.WriteCellOpts(cell.FgColor(cell.ColorYellow))
			w.topList.Write(l, opt)
		} else {
			w.topList.Write(l)
		}
	}
}

// periodic executes the provided closure periodically every interval.
// Exits when the context expires.
func periodic(ctx context.Context, interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fn()
		case <-ctx.Done():
			return
		}
	}
}

func newLogScroll(ctx context.Context) (*text.Text, error) {
	t, err := text.New(text.RollContent())
	if err != nil {
		return nil, err
	}
	return t, nil
}

func newTopList(ctx context.Context) (*text.Text, error) {
	// FIXME: implement method list roll content
	t, err := text.New(text.RollContent())
	if err != nil {
		return nil, err
	}
	return t, nil
}

func newSizePlot(ctx context.Context) (*linechart.LineChart, error) {
	p, err := linechart.New(
		linechart.YAxisFormattedValues(linechart.ValueFormatterRound),
		linechart.AxesCellOpts(cell.FgColor(cell.ColorBlue)),
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newLatencyPlot(ctx context.Context) (*linechart.LineChart, error) {
	p, err := linechart.New(
		linechart.YAxisFormattedValues(linechart.ValueFormatterRoundWithSuffix(" µs")),
		linechart.AxesCellOpts(cell.FgColor(cell.ColorBlue)),
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}
func newTimePlot(ctx context.Context) (*linechart.LineChart, error) {
	p, err := linechart.New(
		linechart.YAxisFormattedValues(linechart.ValueFormatterRoundWithSuffix(" µs")),
		linechart.AxesCellOpts(cell.FgColor(cell.ColorBlue)),
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// newWidgets creates all widgets used.
func newWidgets(ctx context.Context, c *container.Container) (*widgets, error) {

	topList, err := newTopList(ctx)
	if err != nil {
		return nil, err
	}
	logScroll, err := newLogScroll(ctx)
	if err != nil {
		return nil, err
	}
	sizePlot, err := newSizePlot(ctx)
	if err != nil {
		return nil, err
	}
	latencyPlot, err := newLatencyPlot(ctx)
	if err != nil {
		return nil, err
	}
	timePlot, err := newTimePlot(ctx)
	if err != nil {
		return nil, err
	}
	return &widgets{
		topList:     topList,
		logScroll:   logScroll,
		sizePlot:    sizePlot,
		latencyPlot: latencyPlot,
		timePlot:    timePlot,
	}, nil

}

func gridLayout(w *widgets, layout layoutType) ([]container.Option, error) {

	var elements []grid.Element

	switch layout {
	case layoutTop:
		elements = []grid.Element{
			grid.Widget(w.topList,
				container.Border(linestyle.Light),
				container.BorderTitle("Most used methods"),
			),
		}
	case layoutTopTrace:
		elements = []grid.Element{
			grid.ColWidthPerc(50,
				grid.Widget(w.topList,
					container.Border(linestyle.Light),
					container.BorderTitle("Most used methods"),
				),
			),
			grid.ColWidthPerc(50,
				grid.RowHeightPerc(33,
					grid.Widget(w.latencyPlot,
						container.Border(linestyle.Light),
						container.BorderTitle("Latency (microseconds): reply (yellow), error (red)"),
						container.BorderTitleAlignRight(),
					),
				),
				grid.RowHeightPerc(33,
					grid.Widget(w.timePlot,
						container.Border(linestyle.Light),
						container.BorderTitle("CPU time: user (green), system (yellow)"),
						container.BorderTitleAlignRight(),
					),
				),
				grid.RowHeightPerc(33,
					grid.Widget(w.sizePlot,
						container.Border(linestyle.Light),
						container.BorderTitle("Messages: call size (green), response size (yellow)"),
						container.BorderTitleAlignRight(),
					),
				),
			),
		}
	case layoutTopTraceLogs:
		elements = []grid.Element{
			grid.ColWidthPerc(50,
				grid.RowHeightPerc(50,
					grid.Widget(w.topList,
						container.Border(linestyle.Light),
						container.BorderTitle("Most used methods"),
					),
				),
				grid.RowHeightPerc(50,
					grid.Widget(w.logScroll,
						container.Border(linestyle.Light),
						container.BorderTitle("Process logs"),
					),
				),
			),
			grid.ColWidthPerc(50,
				grid.RowHeightPerc(33,
					grid.Widget(w.latencyPlot,
						container.Border(linestyle.Light),
						container.BorderTitle("Latency (microseconds): reply (yellow), error (red)"),
						container.BorderTitleAlignRight(),
					),
				),
				grid.RowHeightPerc(33,
					grid.Widget(w.timePlot,
						container.Border(linestyle.Light),
						container.BorderTitle("CPU time: user (green), system (yellow)"),
						container.BorderTitleAlignRight(),
					),
				),
				grid.RowHeightPerc(33,
					grid.Widget(w.sizePlot,
						container.Border(linestyle.Light),
						container.BorderTitle("Messages: call size (green), response size (yellow)"),
						container.BorderTitleAlignRight(),
					),
				),
			),
		}
	}

	builder := grid.New()
	builder.Add(elements...)
	gridOpts, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return gridOpts, nil
}

// setLayout sets the specified layout.
func setLayout(c *container.Container, w *widgets, lt layoutType) error {
	gridOpts, err := gridLayout(w, lt)
	if err != nil {
		return err
	}
	// remove border: else the previous container border is kept
	c.Update(rootID, container.Border(linestyle.None))
	return c.Update(rootID, gridOpts...)
}

func key(c *container.Container, w *widgets, k *terminalapi.Keyboard) error {
	switch k.Key {
	case 'k', keyboard.KeyArrowUp:
		if w.index > 0 {
			w.index--
		}
		w.updateTopList()
	case 'j', keyboard.KeyArrowDown:
		if w.index < len(w.lines)-1 {
			w.index++
		}
		w.updateTopList()
	case keyboard.KeyEnter:
		if w.index == 0 {
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

		line := w.lines[w.index]
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
		}
	}
	return nil
}

func selectMethod(c *container.Container, w *widgets, service, method string) error {

	if w.collector != nil {
		w.collector.cancel()
		w.collector = nil
	}
	if w.logger != nil {
		w.logger.cancel()
		w.logger = nil
	}

	collector, err := newCollector(sess, w, service, method)
	if err != nil {
		return err
	}
	logger, err := newLogger(sess, w, service, method)
	if err != nil {
		return err
	}
	w.collector = collector
	w.logger = logger
	return nil
}

func run() error {
	var service string
	var method string
	var logFile string

	var level int
	logLevelInfo := "log level, 1:fatal, 2:error, 3:warning, 4:info, 5:verbose, 6:debug"

	flag.StringVar(&service, "service", "", "service name")
	flag.StringVar(&method, "method", "", "method name")
	flag.IntVar(&level, "log-level", 5, logLevelInfo)
	flag.StringVar(&logFile, "log-file", "", "file where to write qitop logs")

	if level < 0 || level > 6 {
		return fmt.Errorf("invalid log level")
	}
	logLevel = qilog.LogLevel{Level: int32(level)}

	flag.Parse()
	var err error
	sess, err = app.SessionFromFlag()
	if err != nil {
		return err
	}

	t, err := termbox.New(termbox.ColorMode(terminalapi.ColorMode256))
	if err != nil {
		return err
	}
	defer t.Close()

	log.SetFlags(0)
	logger := ioutil.Discard
	if logFile != "" {
		var flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		logger, err = os.OpenFile(logFile, flag, 0600)
		if err != nil {
			return err
		}
	}
	defer log.SetOutput(log.Writer())
	log.SetOutput(logger)

	c, err := container.New(t, container.ID(rootID))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := newWidgets(ctx, c)
	if err != nil {
		return err
	}

	updater, err := statUpdater(ctx, sess, cancel)
	if err != nil {
		return err
	}

	go periodic(ctx, 1*time.Second, func() {
		lines, err := updater()
		if err != nil {
			mainErr = err
			cancel()
		}
		w.refreshTopList(lines)
	})

	if service != "" && method != "" {
		selectMethod(c, w, service, method)
		if err != nil {
			return err
		}
		setLayout(c, w, layoutTopTraceLogs)
	} else {
		setLayout(c, w, layoutTop)
	}
	if err != nil {
		return err
	}

	quitter := func(k *terminalapi.Keyboard) {
		err := key(c, w, k)
		if err != nil {
			mainErr = err
			cancel()
		}
		switch k.Key {
		case keyboard.KeyEsc, keyboard.KeyCtrlC:
			cancel()
		}
	}

	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter),
		termdash.RedrawInterval(redrawInterval)); err != nil {
		return err
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	if mainErr != nil {
		log.Fatal(mainErr)
	}
}
