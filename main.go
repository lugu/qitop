package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lugu/qiloop/app"
	"github.com/lugu/qiloop/bus"
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

var (
	sess bus.Session
)

// widgets holds the widgets used by this demo.
type widgets struct {
	sizePlot    *linechart.LineChart
	latencyPlot *linechart.LineChart
	topList     *text.Text

	index   int
	lines   []string
	counter []entry

	collector *collector
}

func (w *widgets) key(k *terminalapi.Keyboard) error {
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
			return nil
		}
		line := w.lines[w.index]
		labels := strings.SplitN(line, " | ", 5)
		if len(labels) != 5 {
			return fmt.Errorf("invalid line: %s", line)
		}
		desc := strings.SplitN(labels[4], ".", 2)
		if len(desc) != 2 {
			return fmt.Errorf("invalid service.action: %s", labels[4])
		}
		if w.collector != nil {
			w.collector.cancel()
			w.collector = nil
		}
		collector, err := newCollector(sess, w, desc[0], desc[1])
		if err != nil {
			return err
		}
		w.collector = collector
	}
	return nil
}

func (w *widgets) refreshTopList(lines []string) error {
	w.lines = lines
	w.updateTopList()
	return nil
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
func periodic(ctx context.Context, interval time.Duration, fn func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := fn(); err != nil {
				panic(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func newTopList(ctx context.Context) (*text.Text, error) {
	t, err := text.New(text.RollContent())
	if err != nil {
		return nil, err
	}
	return t, nil
}

func newSizePlot(ctx context.Context) (*linechart.LineChart, error) {
	p, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorGreen)),
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newLatencyPlot(ctx context.Context) (*linechart.LineChart, error) {
	p, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorGreen)),
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
	sizePlot, err := newSizePlot(ctx)
	if err != nil {
		return nil, err
	}
	latencyPlot, err := newLatencyPlot(ctx)
	if err != nil {
		return nil, err
	}
	return &widgets{
		topList:     topList,
		sizePlot:    sizePlot,
		latencyPlot: latencyPlot,
	}, nil

}

func gridLayout(w *widgets) ([]container.Option, error) {

	elements := []grid.Element{
		grid.ColWidthPerc(50,
			grid.Widget(w.topList,
				container.Border(linestyle.Light),
				container.BorderTitle("Top"),
			),
		),
		grid.ColWidthPerc(50,
			grid.RowHeightPerc(50,
				grid.Widget(w.latencyPlot),
			),
			grid.RowHeightPerc(50,
				grid.Widget(w.sizePlot),
			),
		),
	}
	builder := grid.New()
	builder.Add(elements...)
	gridOpts, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return gridOpts, nil
}

func main() {

	flag.Parse()
	var err error
	sess, err = app.SessionFromFlag()
	if err != nil {
		panic(err)
	}

	t, err := termbox.New(termbox.ColorMode(terminalapi.ColorMode256))
	if err != nil {
		panic(err)
	}
	defer t.Close()

	c, err := container.New(t, container.ID(rootID))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w, err := newWidgets(ctx, c)
	if err != nil {
		panic(err)
	}

	updater, err := statUpdater(ctx, sess, cancel)
	if err != nil {
		panic(err)
	}

	go periodic(ctx, 1*time.Second, func() error {
		lines, err := updater()
		if err != nil {
			return err
		}
		return w.refreshTopList(lines)
	})

	gridOpts, err := gridLayout(w) // equivalent to contLayout(w)
	if err != nil {
		panic(err)
	}

	if err := c.Update(rootID, gridOpts...); err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		err := w.key(k)
		if err != nil {
			cancel()
			log.Fatal(err)
		}
		if k.Key == keyboard.KeyEsc || k.Key == keyboard.KeyCtrlC {
			cancel()
		}
	}
	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter),
		termdash.RedrawInterval(redrawInterval)); err != nil {
		panic(err)
	}
}
