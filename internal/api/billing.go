package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Headers Drape sets on API responses while an organization's billing needs attention, before
// anything is blocked.
const (
	billingWarningHeader       = "X-Drape-Billing-Warning"
	billingDaysRemainingHeader = "X-Drape-Billing-Days-Remaining"
)

// PaymentRequiredError is a 402: the organization's Drape access has ended, so the server saved
// nothing. Billing must never fail a customer's CI, so the CLI reports it and exits 0.
type PaymentRequiredError struct {
	Detail     string
	UpgradeURL string
}

func (e *PaymentRequiredError) Error() string {
	return fmt.Sprintf("API error %d: %s", http.StatusPaymentRequired, e.Detail)
}

// BillingWarning is what the warning headers said: the organization's access ends soon.
type BillingWarning struct {
	Kind string
	// DaysRemaining is -1 when the server didn't say.
	DaysRemaining int
}

// What this process heard about billing, from any response: one run makes many requests, and
// commands report failures in their own ways, so the CLI reads it once at exit.
var billing struct {
	sync.Mutex
	paymentRequired *PaymentRequiredError
	warning         *BillingWarning
}

// PaymentRequired returns the 402 a request got during this run, or nil.
func PaymentRequired() *PaymentRequiredError {
	billing.Lock()
	defer billing.Unlock()
	return billing.paymentRequired
}

// Warning returns the billing warning a response carried during this run, or nil.
func Warning() *BillingWarning {
	billing.Lock()
	defer billing.Unlock()
	return billing.warning
}

// ResetBilling forgets what was heard, at the start of a run.
func ResetBilling() {
	billing.Lock()
	defer billing.Unlock()
	billing.paymentRequired = nil
	billing.warning = nil
}

// safeURL returns raw if it is a plain https URL, else "": it is printed where CI runners read
// workflow commands, so a line break in it could inject one.
func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsFunc(raw, unicode.IsControl) {
		return ""
	}
	return raw
}

func recordPaymentRequired(err *PaymentRequiredError) {
	billing.Lock()
	defer billing.Unlock()
	if billing.paymentRequired == nil {
		billing.paymentRequired = err
	}
}

func recordBillingWarning(resp *http.Response) {
	kind := resp.Header.Get(billingWarningHeader)
	if kind == "" {
		return
	}
	days, err := strconv.Atoi(resp.Header.Get(billingDaysRemainingHeader))
	if err != nil {
		days = -1
	}
	billing.Lock()
	defer billing.Unlock()
	billing.warning = &BillingWarning{Kind: kind, DaysRemaining: days}
}
