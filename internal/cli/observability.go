// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newMetricsCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("metrics", "Inspect current runtime metrics without fabricating missing counters")
	root.AddCommand(newMetricsShowCommand(state, open), newMetricsWatchCommand(state, open))
	return root
}

func newMetricsShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the latest collector-backed metrics snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.Metrics(cmd.Context())
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "metrics_read_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, metricsText(result))
		},
	}
}

func newMetricsWatchCommand(state *options, open openApplicationFunc) *cobra.Command {
	var interval time.Duration
	command := &cobra.Command{
		Use:   "watch",
		Short: "Stream collector-backed metrics until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval < 250*time.Millisecond || interval > time.Hour {
				return &Error{Kind: ErrorUsage, Code: "metrics_interval_invalid", Message: "--interval must be between 250ms and 1h"}
			}
			if state.format == outputJSON {
				return &Error{Kind: ErrorUsage, Code: "stream_output_invalid", Message: "metrics watch requires --output text or jsonl"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			for {
				result, err := instance.Metrics(cmd.Context())
				if err != nil {
					return &Error{Kind: ErrorDomain, Code: "metrics_read_failed", Message: err.Error(), Cause: err}
				}
				if err := writeResult(cmd.OutOrStdout(), state.format, result, metricsText(result)); err != nil {
					return err
				}
				timer := time.NewTimer(interval)
				select {
				case <-cmd.Context().Done():
					if !timer.Stop() {
						<-timer.C
					}
					return cmd.Context().Err()
				case <-timer.C:
				}
			}
		},
	}
	command.Flags().DurationVar(&interval, "interval", 2*time.Second, "poll interval (250ms-1h)")
	return command
}

func metricsText(result application.MetricsSnapshot) string {
	if !result.Available || result.CurrentTrafficData == nil {
		return fmt.Sprintf(
			"unavailable\treason=%s\tbundle=%s\ttier=%s\tcollected=%s",
			emptyAsDash(result.ReasonCode), emptyAsDash(result.AppliedBundleID),
			emptyAsDash(string(result.MonitoringTier)), result.CollectedAt.Format(time.RFC3339Nano),
		)
	}
	period := result.CurrentTrafficData
	return fmt.Sprintf(
		"available\tbundle=%s\tperiod=%s\tin=%d\tout=%d\tcollected=%s",
		result.AppliedBundleID, period.ID, period.InboundBytes, period.OutboundBytes,
		result.CollectedAt.Format(time.RFC3339Nano),
	)
}

func newTrafficCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("traffic", "Inspect collector-backed traffic accounting")
	root.AddCommand(newTrafficStatusCommand(state, open))
	period := group("period", "Inspect immutable traffic periods")
	period.AddCommand(newTrafficPeriodListCommand(state, open), newTrafficPeriodShowCommand(state, open))
	root.AddCommand(period)
	return root
}

func newTrafficStatusCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether current applied-bundle traffic data is available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.TrafficStatus(cmd.Context())
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "traffic_status_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, metricsText(result))
		},
	}
}

func newTrafficPeriodListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var bundleID, fromRaw, toRaw string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List persisted traffic periods by overlap and applied bundle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			from, err := optionalRFC3339(fromRaw, "--from")
			if err != nil {
				return err
			}
			to, err := optionalRFC3339(toRaw, "--to")
			if err != nil {
				return err
			}
			if from != nil && to != nil && !to.After(*from) {
				return &Error{Kind: ErrorUsage, Code: "traffic_period_range_invalid", Message: "--to must be later than --from"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			periods, err := instance.ListTrafficPeriods(cmd.Context(), store.TrafficPeriodFilter{
				ActivationBundleID: strings.TrimSpace(bundleID), OverlapsStart: from, OverlapsEnd: to, Limit: limit,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "traffic_period_filter_invalid", Message: err.Error(), Cause: err}
			}
			var text strings.Builder
			for _, period := range periods {
				fmt.Fprintf(
					&text, "%s\t%s\t%s\t%d\t%d\n",
					period.ID, period.PeriodStart.Format(time.RFC3339Nano),
					period.PeriodEnd.Format(time.RFC3339Nano), period.InboundBytes, period.OutboundBytes,
				)
			}
			plain := strings.TrimSuffix(text.String(), "\n")
			if plain == "" {
				plain = "no traffic periods"
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"items": periods}, plain)
		},
	}
	command.Flags().StringVar(&bundleID, "bundle", "", "filter by immutable activation bundle ID")
	command.Flags().StringVar(&fromRaw, "from", "", "include periods ending after this RFC3339 instant")
	command.Flags().StringVar(&toRaw, "to", "", "include periods starting before this RFC3339 instant")
	command.Flags().IntVar(&limit, "limit", 50, "maximum periods to return (1-200)")
	return command
}

func newTrafficPeriodShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show PERIOD_ID",
		Short: "Show one persisted traffic period",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			period, err := instance.TrafficPeriod(cmd.Context(), args[0])
			if err != nil {
				if application.IsTrafficPeriodNotFound(err) {
					return &Error{Kind: ErrorDomain, Code: "traffic_period_not_found", Message: err.Error(), Cause: err}
				}
				return &Error{Kind: ErrorDomain, Code: "traffic_period_read_failed", Message: err.Error(), Cause: err}
			}
			text := fmt.Sprintf("%s\t%s\t%s\t%d\t%d", period.ID, period.PeriodStart.Format(time.RFC3339Nano), period.PeriodEnd.Format(time.RFC3339Nano), period.InboundBytes, period.OutboundBytes)
			return writeResult(cmd.OutOrStdout(), state.format, period, text)
		},
	}
}

func optionalRFC3339(raw, flag string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, &Error{Kind: ErrorUsage, Code: "time_invalid", Message: flag + " must be an RFC3339 instant", Cause: err}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
