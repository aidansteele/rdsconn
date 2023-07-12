package main

import (
	"context"
	"fmt"
	"github.com/aidansteele/rdsconn/ec2ic"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"io"
	"net"
	"sort"
	"time"
)

func runProxy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if len(args) > 1 {
		return fmt.Errorf("expected either zero or one non-flag arguments: the rds instance id. %d provided", len(args))
	}

	f := cmd.PersistentFlags()
	endpointId, _ := f.GetString("endpoint-id")
	localPort, _ := f.GetInt("local-port")

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	rdsapi := rds.NewFromConfig(cfg)
	var instance rdstypes.DBInstance

	if len(args) == 1 {
		instanceId := args[0]
		describeDBs, err := rdsapi.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: &instanceId})
		if err != nil {
			return fmt.Errorf(": %w", err)
		}

		instance = describeDBs.DBInstances[0]
	} else {
		instance, err = promptForInstanceId(ctx, rdsapi)
		if err != nil {
			return fmt.Errorf("prompting for instance id: %w", err)
		}
	}

	if endpointId == "" {
		vpcId := *instance.DBSubnetGroup.VpcId

		ec2api := ec2.NewFromConfig(cfg)
		describe, err := ec2api.DescribeInstanceConnectEndpoints(ctx, &ec2.DescribeInstanceConnectEndpointsInput{
			Filters: []types.Filter{{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcId},
			}},
		})
		if err != nil {
			return fmt.Errorf(": %w", err)
		}

		if len(describe.InstanceConnectEndpoints) == 0 {
			return fmt.Errorf("no instance connect endpoints found for vpc %s", vpcId)
		}

		endpoint := describe.InstanceConnectEndpoints[0]
		endpointId = *endpoint.InstanceConnectEndpointId
	}

	dialer, err := ec2ic.NewDialer(ctx, cfg, endpointId, time.Hour)

	listener, localPort, err := listenerAndPort(localPort, int(instance.Endpoint.Port))
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Proxy running. Now waiting to serve connections to localhost:%d...\n", localPort)

	for {
		local, err := listener.Accept()
		if err != nil {
			return fmt.Errorf(": %w", err)
		}

		remote, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", *instance.Endpoint.Address, instance.Endpoint.Port))
		if err != nil {
			return fmt.Errorf(": %w", err)
		}

		go serve(ctx, local, remote)
	}

	return nil
}

func promptForInstanceId(ctx context.Context, api *rds.Client) (rdstypes.DBInstance, error) {
	instances := map[string]rdstypes.DBInstance{}
	ids := []string{}

	p := rds.NewDescribeDBInstancesPaginator(api, &rds.DescribeDBInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return rdstypes.DBInstance{}, fmt.Errorf("paginating rds instances: %w", err)
		}

		for _, instance := range page.DBInstances {
			id := *instance.DBInstanceIdentifier
			instances[id] = instance
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	prompt := promptui.Select{
		Label: "Select an RDS instance",
		Items: ids,
	}

	_, instanceId, err := prompt.Run()
	if err != nil {
		return rdstypes.DBInstance{}, fmt.Errorf("prompting user for rds instance: %w", err)
	}

	return instances[instanceId], nil
}

func listenerAndPort(localPort, remotePort int) (net.Listener, int, error) {
	if localPort != 0 {
		l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", localPort))
		return l, localPort, err
	}

	for port := remotePort; port < remotePort+100; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			return l, port, nil
		}
	}

	return nil, 0, fmt.Errorf("unable to allocate local port")
}

func serve(ctx context.Context, local, remote net.Conn) {
	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		_, err := io.Copy(local, remote)
		if err != nil {
			return fmt.Errorf("remote->local: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		_, err := io.Copy(remote, local)
		if err != nil {
			return fmt.Errorf("local->remote: %w", err)
		}

		return nil
	})

	err := g.Wait()
	if err != nil {
		fmt.Println(fmt.Sprintf("%+v", err))
	}
}
