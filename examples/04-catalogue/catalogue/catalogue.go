// Package catalogue is ordinary Go modelled the way Go models things: a named
// type for a fixed set of states, a value type whose meaning is its text, an
// embedded struct carrying shared fields, and an interface for what it needs
// from outside.
package catalogue

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Status is an enum: a named integer, a block of constants, and a String
// method. It crosses as the set of values it can take.
type Status int

const (
	StatusDraft Status = iota
	StatusPublished
	StatusArchived
)

func (s Status) String() string {
	switch s {
	case StatusDraft:
		return "draft"
	case StatusPublished:
		return "published"
	case StatusArchived:
		return "archived"
	}

	return "unknown"
}

// Money is a value whose meaning is its text rather than its field. The
// manifest maps it, so JavaScript sees "12.34" and never learns about cents.
type Money struct {
	Cents int
}

// MoneyToText is the way out. Its signature is the whole declaration: Money
// crosses as a string.
func MoneyToText(m Money) string {
	part := strconv.Itoa(m.Cents % 100)
	if len(part) == 1 {
		part = "0" + part
	}

	return strconv.Itoa(m.Cents/100) + "." + part
}

// MoneyFromText is the way back, and says what it cannot read.
func MoneyFromText(text string) (Money, error) {
	whole, part, found := strings.Cut(text, ".")
	if !found || len(part) != 2 {
		return Money{}, errors.New("expected an amount like 12.34")
	}

	units, err := strconv.Atoi(whole)
	if err != nil {
		return Money{}, errors.New("expected an amount like 12.34")
	}

	cents, err := strconv.Atoi(part)
	if err != nil {
		return Money{}, errors.New("expected an amount like 12.34")
	}

	return Money{Cents: units*100 + cents}, nil
}

// Audited is embedded rather than repeated. Its method is promoted onto
// whatever embeds it, in JavaScript as in Go.
type Audited struct {
	CreatedAt time.Time
}

// Listed reports when the item was listed, in words.
func (a Audited) Listed() string {
	return a.CreatedAt.UTC().Format("2 Jan 2006")
}

// Item embeds Audited, so Listed is callable on an Item.
type Item struct {
	Audited

	Name   string
	Price  Money
	Status Status
}

// Reprice takes the amount as text, because that is how Money crosses.
func (i *Item) Reprice(to Money) {
	i.Price = to
}

// Notifier is what the catalogue needs from outside. JavaScript supplies it.
type Notifier interface {
	Notify(message string)
}

var items = []*Item{
	{Audited: Audited{CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}, Name: "Anvil", Price: Money{Cents: 4999}, Status: StatusPublished},
	{Audited: Audited{CreatedAt: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)}, Name: "Rope", Price: Money{Cents: 1250}, Status: StatusDraft},
	{Audited: Audited{CreatedAt: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)}, Name: "Lantern", Price: Money{Cents: 875}, Status: StatusArchived},
}

// All hands out the catalogue, each item a live view of the Go value.
func All() []*Item {
	return items
}

// Describe renders an item, reading the fields JavaScript may have written.
func Describe(i *Item) string {
	return i.Name + " (" + i.Status.String() + ") at " + MoneyToText(i.Price)
}

// Restock tells the notifier about everything it touched, and reports how many
// that was. The notifier came from JavaScript.
func Restock(notify Notifier, names []string) int {
	touched := 0

	for _, name := range names {
		for _, item := range items {
			if item.Name != name {
				continue
			}

			item.Status = StatusPublished
			notify.Notify("restocked " + item.Name)

			touched++
		}
	}

	return touched
}
