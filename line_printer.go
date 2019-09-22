package main

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

type FragmentColor int

const (
	NoneColor FragmentColor = iota
	YellowColor
	RedColor
	BlueColor
	GreenColor
	WhiteColor
)

var (
	yellowFn = color.New(color.FgYellow).SprintFunc()
	redFn    = color.New(color.FgRed).SprintFunc()
	blueFn   = color.New(color.FgBlue).SprintFunc()
	greenFn  = color.New(color.FgGreen).SprintFunc()
	whiteFn  = color.New(color.FgWhite).SprintFunc()
)

type LinePrinter struct {
	fragments     []string
	currentLength int
	maxLength     int
}

func NewLinePrinter(maxLength int) *LinePrinter {
	return &LinePrinter{
		fragments:     []string{},
		currentLength: 0,
		maxLength:     maxLength,
	}
}

func (p *LinePrinter) Length() int {
	l := p.currentLength + len(p.fragments) - 1
	if l < 0 {
		return 0
	}
	return l
}

// trims whitespace
func (p *LinePrinter) Add(fragment string) bool {
	return p.AddColor(NoneColor, strings.TrimSpace(fragment))
}

func (p *LinePrinter) Addf(format string, a ...interface{}) bool {
	return p.AddColor(NoneColor, fmt.Sprintf(format, a...))
}

func (p *LinePrinter) AddColor(color FragmentColor, fragment string) bool {
	length := len(fragment)
	if length <= 0 {
		return false
	}
	nextLength := p.Length() + length
	if nextLength < p.maxLength {
		p.fragments = append(p.fragments, ColorStr(color, fragment))
		p.currentLength += length
		return true
	}
	return false
}

func (p *LinePrinter) AddColorf(color FragmentColor, format string, a ...interface{}) bool {
	return p.AddColor(color, fmt.Sprintf(format, a...))
}

func (p *LinePrinter) AddFields(str string) int {
	return p.AddFieldsColor(NoneColor, str)
}

func (p *LinePrinter) AddFieldsColor(color FragmentColor, str string) int {
	n := 0
	for _, field := range strings.Fields(str) {
		if p.AddColor(color, field) {
			n += 1
		} else {
			return n
		}
	}
	return n
}

func (p *LinePrinter) String() string {
	return strings.Join(p.fragments, " ")
}

func ColorStr(color FragmentColor, str string) string {
	switch color {
	case YellowColor:
		return yellowFn(str)
	case RedColor:
		return redFn(str)
	case BlueColor:
		return blueFn(str)
	case GreenColor:
		return greenFn(str)
	case WhiteColor:
		return whiteFn(str)
	default:
		return str
	}
}
