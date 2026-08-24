// Package ledger keeps a running balance in the browser, and knows nothing
// about JavaScript.
package ledger

import "strconv"

// Store is the two methods this app needs from the browser's storage, not the
// whole Web Storage API. Declaring what is called rather than what exists is
// what keeps an imported surface small.
type Store interface {
	GetItem(key string) string
	SetItem(key string, value string)
}

// Saved is filled at start-up from globalThis.localStorage. Nothing here knows
// that: it is an ordinary interface, and Go calls it as one.
var Saved Store

const (
	balanceKey = "crystalline.ledger.balance"
	nextIDKey  = "crystalline.ledger.next"

	// Entries are numbered from past where a JavaScript number stops counting
	// exactly, which is the whole reason the field carries a bigint.
	firstID = 9007199254740993
)

// Entry is one line of the ledger.
type Entry struct {
	// ID is exact on both sides. Without the tag it would arrive as a double
	// and 9007199254740993 would read back as ...992.
	ID int64 `crystalline:"bigint"`

	// Note is called description in JavaScript. An explicit name wins over the
	// naming convention, which would have made this one "note".
	Note string `crystalline:"name=description"`

	Amount int
}

// Add records an entry and returns the ledger after it.
func Add(note string, amount int) Entry {
	entry := Entry{ID: nextID(), Note: note, Amount: amount}

	Saved.SetItem(nextIDKey, strconv.FormatInt(entry.ID+1, 10))
	Saved.SetItem(balanceKey, strconv.Itoa(Balance()+amount))

	return entry
}

// Balance reads the running total back out of the browser.
func Balance() int {
	stored, err := strconv.Atoi(Saved.GetItem(balanceKey))
	if err != nil {
		return 0
	}

	return stored
}

// Forget clears the ledger, so the page can be run again from nothing.
func Forget() {
	Saved.SetItem(balanceKey, "0")
	Saved.SetItem(nextIDKey, strconv.FormatInt(firstID, 10))
}

func nextID() int64 {
	stored, err := strconv.ParseInt(Saved.GetItem(nextIDKey), 10, 64)
	if err != nil {
		return firstID
	}

	return stored
}
