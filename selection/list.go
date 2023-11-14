package selection

import (
	"errors"
	"fmt"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/private/canvas"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/text"

	tb "github.com/nsf/termbox-go"
)

// SelectionList displays a list of item which can be selected.
//
// Each line represents an actionable item.
//
// Implements widgetapi.Widget. This object is thread-safe.
type SelectionList struct {
	*text.Text
	onSelect func(int, string) error
	items    []string
	current  int
	first    int
}

func New() (*SelectionList, error) {
	t, err := text.New()
	if err != nil {
		return nil, err
	}
	return &SelectionList{
		t,
		func(int, string) error { return errors.New("not configured") },
		[]string{},
		0,
		0,
	}, nil
}

func (s *SelectionList) updateUI() {
	s.Reset()
	for i, item := range s.items[s.first:] {
		l := fmt.Sprintf("%s\n", item)
		if i+s.first == s.current {
			opt := text.WriteCellOpts(cell.FgColor(cell.ColorYellow))
			s.Write(l, opt)
		} else {
			s.Write(l)
		}
	}
}

func (s *SelectionList) Draw(cvs *canvas.Canvas, meta *widgetapi.Meta) error {
	panic("Not yet implemented")
}

func (s *SelectionList) Mouse(m *terminalapi.Mouse, meta *widgetapi.EventMeta) error {
	panic("Not yet implemented")
}

func (s *SelectionList) Configure(items []string, onSelect func(int, string) error) {
	s.items = items
	s.onSelect = onSelect
	s.updateUI()
}

func (s *SelectionList) Keyboard(k *terminalapi.Keyboard, meta *widgetapi.EventMeta) error {
	switch k.Key {
	case 'k', keyboard.KeyArrowUp:
		if s.current > 0 {
			s.current--
			if s.first > 0 && s.current < s.first+2 {
				s.first--
			}
		}
		s.updateUI()
	case 'j', keyboard.KeyArrowDown:
		if s.current < len(s.items)-1 {
			s.current++
			_, heigh := tb.Size()
			heigh = heigh/2 - 6
			if s.first+heigh < s.current {
				s.first++
			}
		}
		s.updateUI()
	case keyboard.KeyEnter:
		index, item := s.current, s.items[s.current]
		return s.onSelect(index, item)
	}
	return nil
}

func (s *SelectionList) Options() widgetapi.Options {
	opt := s.Text.Options()
	opt.WantKeyboard = widgetapi.KeyScopeGlobal
	return opt
}
