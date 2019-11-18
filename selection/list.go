package selection

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/text"
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
	mu       sync.Mutex
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
		sync.Mutex{},
	}, nil
}

func (s *SelectionList) updateUI() {
	s.Reset()
	for i, item := range s.items {
		l := fmt.Sprintf("%s\n", item)
		if i == s.current {
			opt := text.WriteCellOpts(cell.FgColor(cell.ColorYellow))
			s.Write(l, opt)
		} else {
			s.Write(l)
		}
	}
}

func (s *SelectionList) Configure(items []string, onSelect func(int, string) error) {
	s.items = items
	s.onSelect = onSelect
	s.updateUI()
}

func (s *SelectionList) Keyboard(k *terminalapi.Keyboard) error {
	switch k.Key {
	case 'k', keyboard.KeyArrowUp:
		if s.current > 0 {
			s.current--
		}
		s.updateUI()
	case 'j', keyboard.KeyArrowDown:
		if s.current < len(s.items)-1 {
			s.current++
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
