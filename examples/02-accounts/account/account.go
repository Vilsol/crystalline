// Package account shows what crystalline does with a struct: the value stays in
// Go, and JavaScript gets a live view of it.
package account

import (
	"errors"
	"strconv"
	"strings"
)

// Account is exposed as an interface whose fields read and write through to the
// Go value behind them, with its methods bound alongside.
type Account struct {
	Owner   string
	Balance int

	// A nil slice would arrive as null. The tag makes it an empty array
	// instead, and the field non-optional in the generated declarations.
	History []string `crystalline:"not_nil"`
}

// Open creates an account. Returning a pointer means every wrapper JavaScript
// holds refers to the same Go value.
func Open(owner string, opening int) *Account {
	account := &Account{Owner: owner, Balance: opening}
	account.record("opened", opening)

	return account
}

// Deposit returns only an error, which reaches JavaScript as a Result<void>.
func (a *Account) Deposit(amount int) error {
	if amount <= 0 {
		return errors.New("deposit must be positive")
	}

	a.Balance += amount
	a.record("deposit", amount)

	return nil
}

// Withdraw fails when the money is not there. Failing is not the same as being
// slow, so the call stays synchronous and carries the failure in its Result.
func (a *Account) Withdraw(amount int) error {
	if amount <= 0 {
		return errors.New("withdrawal must be positive")
	}

	if amount > a.Balance {
		return errors.New("insufficient funds: balance is " + strconv.Itoa(a.Balance))
	}

	a.Balance -= amount
	a.record("withdraw", amount)

	return nil
}

// Statement reads the fields, so it reflects anything JavaScript wrote to them.
func (a *Account) Statement() string {
	return a.Owner + ": " + strconv.Itoa(a.Balance) + " (" + strings.Join(a.History, ", ") + ")"
}

// Audit is exported for Go callers but kept off the JavaScript surface by the
// manifest, so the generated declarations do not mention it.
func (a *Account) Audit() string {
	return strings.Join(a.History, "\n")
}

// record is unexported, so it never reaches JavaScript at all.
func (a *Account) record(kind string, amount int) {
	a.History = append(a.History, kind+" "+strconv.Itoa(amount))
}

// Transfer takes two wrappers. Each resolves back to the Go value it came from
// rather than to a copy, so both accounts really move.
func Transfer(from *Account, to *Account, amount int) error {
	if err := from.Withdraw(amount); err != nil {
		return err
	}

	return to.Deposit(amount)
}

// Summarise takes an account by value, so JavaScript may pass either a wrapper
// or a plain object literal with the same fields.
func Summarise(a Account) string {
	return a.Owner + " holds " + strconv.Itoa(a.Balance)
}

// ParseAmount returns a value and an error: a Result<number> on the other side.
func ParseAmount(text string) (int, error) {
	amount, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, errors.New(strconv.Quote(text) + " is not a whole number")
	}

	return amount, nil
}
