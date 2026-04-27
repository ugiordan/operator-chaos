package reporter

import (
	"encoding/xml"
	"fmt"
	"io"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
)

// JUnitReporter writes experiment reports in JUnit XML format,
// suitable for CI systems like Jenkins and GitHub Actions.
type JUnitReporter struct {
	writer io.Writer
}

type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
	SystemErr string        `xml:"system-err,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// NewJUnitReporter creates a JUnitReporter that writes to the given writer.
func NewJUnitReporter(w io.Writer) *JUnitReporter {
	return &JUnitReporter{writer: w}
}

// WriteSuite writes a collection of experiment reports as a JUnit XML test suite.
func (r *JUnitReporter) WriteSuite(name string, reports []ExperimentReport) error {
	suite := junitTestSuite{
		Name:  name,
		Tests: len(reports),
	}

	var totalSeconds float64
	for _, report := range reports {
		caseSeconds := report.Evaluation.RecoveryTime.Seconds()
		totalSeconds += caseSeconds

		className := fmt.Sprintf("chaos.%s", report.Target.Component)
		if report.Tier > 0 {
			className = fmt.Sprintf("chaos.T%d.%s", report.Tier, report.Target.Component)
		}
		tc := junitTestCase{
			Name:      report.Experiment,
			ClassName: className,
			Time:      fmt.Sprintf("%.3f", caseSeconds),
		}

		switch report.Evaluation.Verdict {
		case v1alpha1.Failed:
			suite.Failures++
			tc.Failure = &junitFailure{
				Message: "Chaos experiment failed",
				Type:    "FAILED",
				Body:    report.Evaluation.Confidence,
			}
		case v1alpha1.Degraded:
			suite.Failures++
			tc.Failure = &junitFailure{
				Message: "System degraded under chaos",
				Type:    "DEGRADED",
				Body:    report.Evaluation.Confidence,
			}
		case v1alpha1.Inconclusive:
			tc.Skipped = &junitSkipped{
				Message: "Could not establish baseline: " + report.Evaluation.Confidence,
			}
		}

		if report.CleanupError != "" {
			tc.SystemErr = report.CleanupError
		}

		suite.Cases = append(suite.Cases, tc)
	}

	suite.Time = fmt.Sprintf("%.3f", totalSeconds)
	suites := junitTestSuites{Suites: []junitTestSuite{suite}}

	output, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}

	_, err = r.writer.Write([]byte(xml.Header))
	if err != nil {
		return err
	}
	_, err = r.writer.Write(output)
	return err
}
