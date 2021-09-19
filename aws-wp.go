package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

func main() {
	defer duration(time.Now())
	imageId := flag.String("ami", "", "The image id for the instance")
	flag.Parse()

	if *imageId == "" {
		fmt.Println("You must supply an AMI")
		return
	}

	client := createClient()

	instanceId := createInstance(client, *imageId)

	publicDnsName := waitRunning(client, instanceId)

	if publicDnsName != "" {
		openBrowser(publicDnsName)
	}
}

func createClient() *ec2.Client {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic("Configuration error, " + err.Error())
	}
	return ec2.NewFromConfig(cfg)
}

func createInstance(client *ec2.Client, imageId string) string {

	securityGroupId := getSecurityGroup(client)

	instancesInput := &ec2.RunInstancesInput{
		ImageId:          aws.String(imageId),
		InstanceType:     types.InstanceTypeT2Micro,
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		SecurityGroupIds: []string{securityGroupId},
	}

	result, err := client.RunInstances(context.TODO(), instancesInput)

	if err != nil {
		fmt.Println("Got an error creating an instance:")
		fmt.Println(err)
		return ""
	}

	instanceId := *result.Instances[0].InstanceId

	setTagName(client, instanceId)

	return instanceId
}

func getSecurityGroup(client *ec2.Client) string {
	var groupName string = "wordpress-sg"
	describeSecurityGroupsInput := &ec2.DescribeSecurityGroupsInput{
		GroupNames: []string{groupName},
	}
	describeSecurityGroup, err := client.DescribeSecurityGroups(context.TODO(), describeSecurityGroupsInput)

	if err == nil && len(describeSecurityGroup.SecurityGroups) > 0 {
		return *describeSecurityGroup.SecurityGroups[0].GroupId
	}

	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() != "InvalidGroup.NotFound" {
				fmt.Println("Got an error retrieving information about security griop:")
				fmt.Println(groupName)
				fmt.Println(err)
				return ""
			}
		}
	}

	sgInput := &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(groupName),
		Description: aws.String("Security group for wordpress"),
	}

	securityGroup, err := client.CreateSecurityGroup(context.TODO(), sgInput)

	if err != nil {
		fmt.Println("Got an error creating an security group:")
		fmt.Println(err)
		return ""
	}

	permissions := []types.IpPermission{
		types.IpPermission{
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(80),
			IpProtocol: aws.String("tcp"),
			IpRanges: []types.IpRange{
				types.IpRange{
					CidrIp: aws.String("0.0.0.0/0"),
				},
			},
			Ipv6Ranges: []types.Ipv6Range{
				types.Ipv6Range{
					CidrIpv6: aws.String("::/0"),
				},
			},
		},
	}

	sgIngressInput := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       securityGroup.GroupId,
		IpPermissions: permissions,
	}

	client.AuthorizeSecurityGroupIngress(context.TODO(), sgIngressInput)

	return *securityGroup.GroupId
}

func setTagName(client *ec2.Client, instanceId string) {
	tagInput := &ec2.CreateTagsInput{
		Resources: []string{instanceId},
		Tags: []types.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("WordPress"),
			},
		},
	}

	_, err := client.CreateTags(context.TODO(), tagInput)

	if err != nil {
		fmt.Println("Got an error tagging the instance:")
		fmt.Println(err)
	}
}

func waitRunning(client *ec2.Client, instanceId string) string {
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	for {
		result, err := client.DescribeInstances(context.TODO(), input)

		if err != nil {
			fmt.Println("Got an error retrieving information about your Amazon EC2 instances:")
			fmt.Println(err)
			return ""
		}

		for _, r := range result.Reservations {
			for _, i := range r.Instances {
				// running
				if *i.State.Code == 16 {
					time.Sleep(7 * time.Second)
					return "http://" + *i.PublicDnsName
				}
				// not pending
				if *i.State.Code != 0 {
					fmt.Println("Got an error creating an instance")
					return ""
				}
				log.Printf("Still pending...")
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Println("Got an error open browser:")
		fmt.Println(err)
		log.Fatal(err)
	}
}

func duration(start time.Time) {
	log.Printf("Start-up time: %v\n", time.Since(start))
}
