package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mote/internal/provider/copilot"
)

// NewUsageCmd creates the usage command.
func NewUsageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "View usage statistics",
		Long: `View Copilot usage statistics and quota status.

Tracks your model usage, including free and premium requests.`,
	}

	cmd.AddCommand(newUsageStatusCmd())
	cmd.AddCommand(newUsageHistoryCmd())
	cmd.AddCommand(newUsageModelsCmd())
	cmd.AddCommand(newUsageResetCmd())

	return cmd
}

func newUsageStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current usage quota status",
		Long:  `Display your current month's usage quota status.`,
		RunE:  runUsageStatus,
	}
}

func newUsageHistoryCmd() *cobra.Command {
	var count int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent usage history",
		Long:  `Display recent usage records.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUsageHistory(cmd, count)
		},
	}

	cmd.Flags().IntVarP(&count, "count", "n", 10, "Number of records to show")

	return cmd
}

func newUsageModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "Show usage by model",
		Long:  `Display usage breakdown by model for the current month.`,
		RunE:  runUsageModels,
	}
}

func newUsageResetCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset usage statistics",
		Long:  `Clear all local usage statistics. This does not affect your actual GitHub quota.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUsageReset(cmd, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runUsageStatus(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	ut := copilot.NewUsageTracker()
	status := ut.GetQuotaStatus()
	monthly := ut.GetCurrentMonthUsage()

	now := time.Now()
	fmt.Printf("Usage Status - %s %d\n", now.Month().String(), now.Year())
	fmt.Println("═══════════════════════════════════")
	fmt.Println("")

	// Free requests
	fmt.Println("Free Requests")
	fmt.Println("-------------")
	fmt.Printf("  Used:      %d / %d\n", status.FreeRequestsUsed, status.FreeRequestsLimit)
	fmt.Printf("  Remaining: %d\n", status.FreeRequestsRemaining)
	fmt.Printf("  Progress:  %.1f%%\n", status.FreePercentUsed)
	fmt.Println("")

	// Premium requests
	fmt.Println("Premium Units")
	fmt.Println("-------------")
	fmt.Printf("  Used:      %d / %d\n", status.PremiumUnitsUsed, status.PremiumUnitsLimit)
	fmt.Printf("  Remaining: %d\n", status.PremiumUnitsRemaining)
	fmt.Printf("  Progress:  %.1f%%\n", status.PremiumPercentUsed)
	fmt.Println("")

	// Token usage
	fmt.Println("Tokens")
	fmt.Println("------")
	fmt.Printf("  Input:     %d\n", monthly.InputTokens)
	fmt.Printf("  Output:    %d\n", monthly.OutputTokens)
	fmt.Printf("  Total:     %d\n", monthly.InputTokens+monthly.OutputTokens)

	return nil
}

func runUsageHistory(cmd *cobra.Command, count int) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	ut := copilot.NewUsageTracker()
	records := ut.GetRecentRecords(count)

	if len(records) == 0 {
		fmt.Println("No usage records found.")
		return nil
	}

	fmt.Printf("Recent Usage (%d records)\n", len(records))
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("")
	fmt.Printf("%-20s %-20s %-8s %-10s %-10s\n", "Time", "Model", "Mode", "Input", "Output")
	fmt.Printf("%-20s %-20s %-8s %-10s %-10s\n", "----", "-----", "----", "-----", "------")

	for _, record := range records {
		timeStr := record.Timestamp.Format("Jan 02 15:04:05")
		modelStr := record.ModelID
		if len(modelStr) > 18 {
			modelStr = modelStr[:15] + "..."
		}

		premiumMarker := ""
		if record.IsPremium {
			premiumMarker = fmt.Sprintf(" (%dx)", record.Multiplier)
		}

		fmt.Printf("%-20s %-20s %-8s %-10d %-10d%s\n",
			timeStr,
			modelStr,
			record.Mode,
			record.InputTokens,
			record.OutputTokens,
			premiumMarker,
		)
	}

	return nil
}

func runUsageModels(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	ut := copilot.NewUsageTracker()
	monthly := ut.GetCurrentMonthUsage()

	if len(monthly.ByModel) == 0 {
		fmt.Println("No usage data for current month.")
		return nil
	}

	now := time.Now()
	fmt.Printf("Usage by Model - %s %d\n", now.Month().String(), now.Year())
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("")
	fmt.Printf("%-25s %-10s %-12s %-12s %-10s\n", "Model", "Requests", "Input", "Output", "Premium")
	fmt.Printf("%-25s %-10s %-12s %-12s %-10s\n", "-----", "--------", "-----", "------", "-------")

	for modelID, usage := range monthly.ByModel {
		modelStr := modelID
		if len(modelStr) > 23 {
			modelStr = modelStr[:20] + "..."
		}

		premiumStr := "-"
		if usage.PremiumUnits > 0 {
			premiumStr = fmt.Sprintf("%d units", usage.PremiumUnits)
		}

		fmt.Printf("%-25s %-10d %-12d %-12d %-10s\n",
			modelStr,
			usage.Requests,
			usage.InputTokens,
			usage.OutputTokens,
			premiumStr,
		)
	}

	fmt.Println("")
	fmt.Printf("Total: %d requests, %d input tokens, %d output tokens\n",
		monthly.TotalRequests,
		monthly.InputTokens,
		monthly.OutputTokens,
	)

	return nil
}

func runUsageReset(cmd *cobra.Command, force bool) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	if !force {
		fmt.Println("⚠️  This will clear all local usage statistics.")
		fmt.Println("   Note: This does not affect your actual GitHub quota.")
		fmt.Print("Continue? (y/N): ")

		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	ut := copilot.NewUsageTracker()
	if err := ut.Reset(); err != nil {
		return fmt.Errorf("failed to reset usage: %w", err)
	}

	fmt.Println("✓ Usage statistics reset successfully.")

	return nil
}
