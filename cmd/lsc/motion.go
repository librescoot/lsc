package lsc

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	ipc "github.com/librescoot/redis-ipc"
	"github.com/spf13/cobra"
)

const motionRPCChannel = "motion:rpc"

type motionEmptyRequest struct{}

type motionCalibrationStatus struct {
	State           string  `json:"state"`
	AcceptedSamples int     `json:"accepted_samples"`
	RejectedSamples int     `json:"rejected_samples"`
	CoverageBins    int     `json:"coverage_bins"`
	RequiredBins    int     `json:"required_bins"`
	SpanX           float64 `json:"span_x"`
	SpanY           float64 `json:"span_y"`
	Ready           bool    `json:"ready"`
	ResidualRMS     float64 `json:"residual_rms,omitempty"`
	Condition       float64 `json:"condition,omitempty"`
	OutputPath      string  `json:"output_path,omitempty"`
	ModelPath       string  `json:"model_path,omitempty"`
}

var motionCmd = &cobra.Command{
	Use:     "motion",
	Short:   "Inspect and calibrate the motion sensors",
	GroupID: "main",
}

var motionCalibrationCmd = &cobra.Command{
	Use:     "calibration",
	Aliases: []string{"calibrate"},
	Short:   "Collect magnetometer calibration data",
}

func motionCalibrationAction(method string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		client, err := newMotionRPCClient()
		if err != nil {
			return err
		}
		defer client.Close()
		status, err := ipc.CallMethod[motionEmptyRequest, motionCalibrationStatus](
			client, motionRPCChannel, method, motionEmptyRequest{}, 5*time.Second)
		if err != nil {
			return fmt.Errorf("motion-service %s: %w", method, err)
		}
		return printMotionCalibrationStatus(cmd, status)
	}
}

func newMotionRPCClient() (*ipc.Client, error) {
	host, portText, err := net.SplitHostPort(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis address %q: %w", redisAddr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis port %q: %w", portText, err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return ipc.New(
		ipc.WithAddress(host), ipc.WithPort(port), ipc.WithPoolSize(2),
		ipc.WithCodec(ipc.JSONCodec{}), ipc.WithLogger(logger),
	)
}

func printMotionCalibrationStatus(cmd *cobra.Command, status motionCalibrationStatus) error {
	if JSONOutput {
		encoded, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return err
	}
	var output string
	output += fmt.Sprintf("Calibration: %s\n", status.State)
	output += fmt.Sprintf("Samples: %d accepted, %d rejected\n",
		status.AcceptedSamples, status.RejectedSamples)
	output += fmt.Sprintf("Coverage: %d/%d bins; spans X %.0f, Y %.0f\n",
		status.CoverageBins, status.RequiredBins, status.SpanX, status.SpanY)
	output += fmt.Sprintf("Ready: %t\n", status.Ready)
	if status.ResidualRMS != 0 {
		output += fmt.Sprintf("Fit: %.1f%% radial RMS; condition %.2f\n",
			status.ResidualRMS*100, status.Condition)
	}
	if status.OutputPath != "" {
		output += fmt.Sprintf("Capture: %s\n", status.OutputPath)
	}
	if status.ModelPath != "" {
		output += fmt.Sprintf("Model: %s\n", status.ModelPath)
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), output)
	return err
}

func init() {
	motionCalibrationCmd.AddCommand(
		&cobra.Command{Use: "start", Short: "Start a new capture", Args: cobra.NoArgs, RunE: motionCalibrationAction("calibration-start")},
		&cobra.Command{Use: "status", Short: "Show capture coverage", Args: cobra.NoArgs, RunE: motionCalibrationAction("calibration-status")},
		&cobra.Command{Use: "finish", Short: "Validate, save, and apply a sufficiently covered capture", Args: cobra.NoArgs, RunE: motionCalibrationAction("calibration-finish")},
		&cobra.Command{Use: "cancel", Short: "Discard the active capture", Args: cobra.NoArgs, RunE: motionCalibrationAction("calibration-cancel")},
		&cobra.Command{Use: "reset", Short: "Delete the calibration and disable magnetic heading", Args: cobra.NoArgs, RunE: motionCalibrationAction("calibration-reset")},
	)
	motionCmd.AddCommand(motionCalibrationCmd)
	rootCmd.AddCommand(motionCmd)
}
