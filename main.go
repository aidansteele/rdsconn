package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		RunE: runList,
	}

	proxyCmd := &cobra.Command{
		Use:     "proxy",
		Aliases: []string{"p"},
		RunE:    runProxy,
	}

	proxyCmd.PersistentFlags().String("endpoint-id", "", "")
	proxyCmd.PersistentFlags().Int("local-port", 0, "")

	rootCmd.AddCommand(proxyCmd)

	ctx := context.Background()
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	rdsapi := rds.NewFromConfig(cfg)

	p := rds.NewDescribeDBInstancesPaginator(rdsapi, &rds.DescribeDBInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fmt.Errorf(": %w", err)
		}

		for _, instance := range page.DBInstances {
			fmt.Fprintln(cmd.OutOrStdout(), *instance.DBInstanceIdentifier)
		}
	}

	return nil
}
