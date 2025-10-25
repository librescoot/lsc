package lsc

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show overall scooter status",
	Long:  `Displays a dashboard of key metrics from various scooter services.`, 
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Fetching scooter status...")

		// Fetch ECU status
		ecuData, err := redisClient.HGetAll("engine-ecu")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching ECU data: %v\n", err)
			return
		}

		// Fetch Battery status (assuming battery:0 for now)
		batteryData, err := redisClient.HGetAll("battery:0")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching Battery data: %v\n", err)
			return
		}

		// Fetch Vehicle state
		vehicleState, err := redisClient.HGet("vehicle", "state")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching Vehicle state: %v\n", err)
			return
		}

		fmt.Println("\n--- Vehicle Status ---")
		fmt.Printf("State: %s\n", vehicleState)
		fmt.Printf("Speed: %s RPM: %s\n", ecuData["speed"], ecuData["rpm"])
		fmt.Printf("Odometer: %s\n", ecuData["odometer"])
		fmt.Printf("Throttle: %s\n", ecuData["throttle"])
		fmt.Printf("Motor Voltage: %s Current: %s\n", ecuData["motor:voltage"], ecuData["motor:current"])
		fmt.Printf("Motor Temperature: %s\n", ecuData["temperature"])

		fmt.Println("\n--- Battery Status (Battery 0) ---")
		fmt.Printf("State: %s Temp State: %s\n", batteryData["state"], batteryData["temperature-state"])
		fmt.Printf("SoC: %s Voltage: %s\n", batteryData["soc"], batteryData["voltage"])
		fmt.Printf("Current: %s Temperature: %s\n", batteryData["current"], batteryData["temperature"])
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
