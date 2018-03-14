// Copyright 2016-2017, Pulumi Corporation.  All rights reserved.

package cmd

import (
	"fmt"
	"time"

	mobytime "github.com/moby/moby/api/types/time"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/pulumi/pulumi/pkg/diag/colors"
	"github.com/pulumi/pulumi/pkg/operations"
	"github.com/pulumi/pulumi/pkg/tokens"
	"github.com/pulumi/pulumi/pkg/util/cmdutil"
)

// We use RFC 5424 timestamps with millisecond precision for displaying time stamps on log entries. Go does not
// pre-define a format string for this format, though it is similar to time.RFC3339Nano.
//
// See https://tools.ietf.org/html/rfc5424#section-6.2.3.
const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func newLogsCmd() *cobra.Command {
	var stack string
	var follow bool
	var since string
	var resource string

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show aggregated logs for a project",
		Args:  cmdutil.NoArgs,
		Run: cmdutil.RunFunc(func(cmd *cobra.Command, args []string) error {
			s, err := requireStack(tokens.QName(stack), false)
			if err != nil {
				return err
			}

			startTime, err := parseSince(since, time.Now())
			if err != nil {
				return errors.Wrapf(err, "failed to parse argument to '--since' as duration or timestamp")
			}
			var resourceFilter *operations.ResourceFilter
			if resource != "" {
				var rf = operations.ResourceFilter(resource)
				resourceFilter = &rf
			}

			fmt.Printf(
				colors.ColorizeText(colors.BrightMagenta+"Collecting logs since %s.\n\n"+colors.Reset),
				startTime.Format(timeFormat),
			)

			// IDEA: This map will grow forever as new log entries are found.  We may need to do a more approximate
			// approach here to ensure we don't grow memory unboundedly while following logs.
			//
			// Note: Just tracking latest log date is not sufficient - as stale logs may show up which should have been
			// displayed before previously rendered log entries, but weren't available at the time, so still need to be
			// rendered now even though they are technically out of order.
			shown := map[operations.LogEntry]bool{}
			for {
				logs, err := s.GetLogs(operations.LogQuery{
					StartTime:      startTime,
					ResourceFilter: resourceFilter,
				})
				if err != nil {
					return errors.Wrapf(err, "failed to get logs")
				}

				for _, logEntry := range logs {
					if _, shownAlready := shown[logEntry]; !shownAlready {
						eventTime := time.Unix(0, logEntry.Timestamp*1000000)
						fmt.Printf("%30.30s[%30.30s] %v\n", eventTime.Format(timeFormat), logEntry.ID, logEntry.Message)
						shown[logEntry] = true
					}
				}

				if !follow {
					return nil
				}

				time.Sleep(time.Second)
			}
		}),
	}

	logsCmd.PersistentFlags().StringVarP(
		&stack, "stack", "s", "",
		"List configuration for a different stack than the currently selected stack")
	logsCmd.PersistentFlags().BoolVarP(
		&follow, "follow", "f", false,
		"Follow the log stream in real time (like tail -f)")
	logsCmd.PersistentFlags().StringVar(
		&since, "since", "1h",
		"Only return logs newer than a relative duration ('5s', '2m', '3h') or absolute timestamp.  "+
			"Defaults to returning the last 1 hour of logs.")
	logsCmd.PersistentFlags().StringVarP(
		&resource, "resource", "r", "",
		"Only return logs for the requested resource ('name', 'type::name' or full URN).  Defaults to returning all logs.")

	return logsCmd
}

func parseSince(since string, reference time.Time) (*time.Time, error) {
	startTimestamp, err := mobytime.GetTimestamp(since, reference)
	if err != nil {
		return nil, err
	}
	startTimeSec, startTimeNs, err := mobytime.ParseTimestamps(startTimestamp, 0)
	if err != nil {
		return nil, err
	}
	if startTimeSec == 0 && startTimeNs == 0 {
		return nil, nil
	}
	startTime := time.Unix(startTimeSec, startTimeNs)
	return &startTime, nil
}
