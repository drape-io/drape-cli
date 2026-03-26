// Package junit parses JUnit XML test result files.
package junit

import (
	"encoding/xml"
	"fmt"
)

// Status represents a test result status.
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

// TestCase represents a single parsed test case.
type TestCase struct {
	Name       string
	ClassName  string
	Suite      string
	File       string
	DurationMs int
	Status     Status
	ErrorMsg   string
	ErrorType  string
}

// Summary holds aggregate counts from a parse result.
type Summary struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errored int
}

// Summarize computes aggregate counts from test cases.
func Summarize(cases []TestCase) Summary {
	s := Summary{Total: len(cases)}
	for _, tc := range cases {
		switch tc.Status {
		case StatusPassed:
			s.Passed++
		case StatusFailed:
			s.Failed++
		case StatusSkipped:
			s.Skipped++
		case StatusError:
			s.Errored++
		}
	}
	return s
}

// Parse parses JUnit XML data and returns a list of test cases.
// It handles both <testsuites> and <testsuite> as root elements.
func Parse(data []byte) ([]TestCase, error) {
	// Try <testsuites> first
	var suites xmlTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.Suites) > 0 {
		return convertSuites(suites.Suites), nil
	}

	// Try single <testsuite>
	var suite xmlTestSuite
	if err := xml.Unmarshal(data, &suite); err == nil && (len(suite.TestCases) > 0 || suite.Name != "") {
		return convertSuites([]xmlTestSuite{suite}), nil
	}

	return nil, fmt.Errorf("failed to parse JUnit XML: no test suites found")
}

func convertSuites(suites []xmlTestSuite) []TestCase {
	var cases []TestCase
	for _, suite := range suites {
		// Handle nested testsuites
		if len(suite.Suites) > 0 {
			cases = append(cases, convertSuites(suite.Suites)...)
		}
		for _, tc := range suite.TestCases {
			testCase := TestCase{
				Name:       tc.Name,
				ClassName:  tc.ClassName,
				Suite:      suite.Name,
				File:       tc.File,
				DurationMs: int(tc.Time * 1000),
				Status:     StatusPassed,
			}
			switch {
			case tc.Failure != nil:
				testCase.Status = StatusFailed
				testCase.ErrorMsg = tc.Failure.Message
				testCase.ErrorType = tc.Failure.Type
			case tc.Error != nil:
				testCase.Status = StatusError
				testCase.ErrorMsg = tc.Error.Message
				testCase.ErrorType = tc.Error.Type
			case tc.Skipped != nil:
				testCase.Status = StatusSkipped
				testCase.ErrorMsg = tc.Skipped.Message
			}
			cases = append(cases, testCase)
		}
	}
	return cases
}

// XML structures for JUnit parsing

type xmlTestSuites struct {
	XMLName xml.Name       `xml:"testsuites"`
	Suites  []xmlTestSuite `xml:"testsuite"`
}

type xmlTestSuite struct {
	XMLName   xml.Name       `xml:"testsuite"`
	Name      string         `xml:"name,attr"`
	Tests     int            `xml:"tests,attr"`
	Failures  int            `xml:"failures,attr"`
	Errors    int            `xml:"errors,attr"`
	Skipped   int            `xml:"skipped,attr"`
	Time      float64        `xml:"time,attr"`
	TestCases []xmlTestCase  `xml:"testcase"`
	Suites    []xmlTestSuite `xml:"testsuite"`
}

type xmlTestCase struct {
	Name      string      `xml:"name,attr"`
	ClassName string      `xml:"classname,attr"`
	File      string      `xml:"file,attr"`
	Time      float64     `xml:"time,attr"`
	Failure   *xmlFailure `xml:"failure"`
	Error     *xmlError   `xml:"error"`
	Skipped   *xmlSkipped `xml:"skipped"`
}

type xmlFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type xmlError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type xmlSkipped struct {
	Message string `xml:"message,attr"`
}
