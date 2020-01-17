package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceAwsPlacementGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsPlacementGroupCreate,
		Read:   resourceAwsPlacementGroupRead,
		Delete: resourceAwsPlacementGroupDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"strategy": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceAwsPlacementGroupCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	name := d.Get("name").(string)
	log.Printf("[INFO] OLIVIER1 Creating EC2 Placement group: %s (step 1)", name)
	input := ec2.CreatePlacementGroupInput{
		GroupName: aws.String(name),
		Strategy:  aws.String(d.Get("strategy").(string)),
	}
	log.Printf("[INFO] OLIVIER1 Creating EC2 Placement group: %s (step 2)", input)
	_, err := conn.CreatePlacementGroup(&input)
	if err != nil {
		log.Printf("[INFO] OLIVIER1 Error creating EC2 Placement group: %s %v", input, err)
		return err
	}

	wait := resource.StateChangeConf{
		Pending:    []string{"pending"},
		Target:     []string{"available"},
		Timeout:    5 * time.Minute,
		MinTimeout: 1 * time.Second,
		Refresh: func() (interface{}, string, error) {
			out, err := conn.DescribePlacementGroups(&ec2.DescribePlacementGroupsInput{
				GroupNames: []*string{aws.String(name)},
			})

			if err != nil {
				log.Printf("[INFO] OLIVIER1 Error describe EC2 Placement group: %q %v", name, err)
				return out, "", err
			}

			if len(out.PlacementGroups) == 0 {
				log.Printf("[INFO] OLIVIER1 Error EC2 Placement group not found: %q", name)
				return out, "", fmt.Errorf("Placement group not found (%q)", name)
			}
			pg := out.PlacementGroups[0]
			log.Printf("[INFO] OLIVIER1 Status creating EC2 Placement group: %q %v", name, *pg.State)

			return out, *pg.State, nil
		},
	}

	_, err = wait.WaitForState()
	if err != nil {
		log.Printf("[INFO] OLIVIER1 Error waiting EC2 Placement group state: %s %v", input, err)
		return err
	}

	log.Printf("[INFO] OLIVIER1 EC2 Placement group created: %q", name)

	d.SetId(name)

	return resourceAwsPlacementGroupRead(d, meta)
}

func resourceAwsPlacementGroupRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[INFO] OLIVIER1 Reading EC2 Placement Group: %s", d.Id())
	conn := meta.(*AWSClient).ec2conn
	input := ec2.DescribePlacementGroupsInput{
		GroupNames: []*string{aws.String(d.Id())},
	}
	out, err := conn.DescribePlacementGroups(&input)
	if err != nil {
		return err
	}
	pg := out.PlacementGroups[0]

	log.Printf("[INFO] OLIVIER1 Received EC2 Placement Group: %s", pg)

	d.Set("name", pg.GroupName)
	d.Set("strategy", pg.Strategy)

	return nil
}

func resourceAwsPlacementGroupDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	log.Printf("[INFO] OLIVIER1 Deleting EC2 Placement Group %q", d.Id())
	_, err := conn.DeletePlacementGroup(&ec2.DeletePlacementGroupInput{
		GroupName: aws.String(d.Id()),
	})
	if err != nil {
		log.Printf("[INFO] OLIVIER1 Error deleting EC2 Placement group: %q %v", d.Id(), err)
		return err
	}

	wait := resource.StateChangeConf{
		Pending:    []string{"deleting"},
		Target:     []string{"deleted"},
		Timeout:    5 * time.Minute,
		MinTimeout: 1 * time.Second,
		Refresh: func() (interface{}, string, error) {
			out, err := conn.DescribePlacementGroups(&ec2.DescribePlacementGroupsInput{
				GroupNames: []*string{aws.String(d.Id())},
			})

			if err != nil {
				awsErr := err.(awserr.Error)
				if awsErr.Code() == "InvalidPlacementGroup.Unknown" {
					log.Printf("[INFO] OLIVIER1 Resetting error deleting EC2 Placement group: %q %v", d.Id(), awsErr)
					return out, "deleted", nil
				}
				log.Printf("[INFO] OLIVIER1 Error waiting deleting EC2 Placement group: %q %v", d.Id(), awsErr)
				return out, "", awsErr
			}

			if len(out.PlacementGroups) == 0 {
				log.Printf("[INFO] OLIVIER1 Not found deleting EC2 Placement group: %q", d.Id())
				return out, "deleted", nil
			}

			pg := out.PlacementGroups[0]
			log.Printf("[INFO] OLIVIER1 Status deleting EC2 Placement group: %q %v", d.Id(), *pg.State)

			return out, *pg.State, nil
		},
	}

	_, err = wait.WaitForState()
	log.Printf("[INFO] OLIVIER1 deleting EC2 Placement group: %q returned %v", d.Id(), err)
	return err
}
