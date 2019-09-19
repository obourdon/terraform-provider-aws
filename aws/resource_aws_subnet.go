package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsSubnet() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsSubnetCreate,
		Read:   resourceAwsSubnetRead,
		Update: resourceAwsSubnetUpdate,
		Delete: resourceAwsSubnetDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(32 * time.Minute),
		},

		SchemaVersion: 1,
		MigrateState:  resourceAwsSubnetMigrateState,

		Schema: map[string]*schema.Schema{
			"vpc_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"cidr_block": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"ipv6_cidr_block": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"availability_zone": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"availability_zone_id"},
			},

			"availability_zone_id": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"availability_zone"},
			},

			"map_public_ip_on_launch": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"assign_ipv6_address_on_creation": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"ipv6_cidr_block_association_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"tags": tagsSchema(),

			"owner_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsSubnetCreate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] OLIVIER1 Subnet create vpc: %v Timeout: %v", d.Get("vpc_id").(string), d.Timeout(schema.TimeoutDelete))
	conn := meta.(*AWSClient).ec2conn

	createOpts := &ec2.CreateSubnetInput{
		AvailabilityZone:   aws.String(d.Get("availability_zone").(string)),
		AvailabilityZoneId: aws.String(d.Get("availability_zone_id").(string)),
		CidrBlock:          aws.String(d.Get("cidr_block").(string)),
		VpcId:              aws.String(d.Get("vpc_id").(string)),
	}

	if v, ok := d.GetOk("ipv6_cidr_block"); ok {
		createOpts.Ipv6CidrBlock = aws.String(v.(string))
	}

	var err error
	resp, err := conn.CreateSubnet(createOpts)

	if err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet create vpc: %v Error creating subnet: %s", d.Get("vpc_id").(string), err)
		return fmt.Errorf("Error creating subnet: %s", err)
	}

	// Get the ID and store it
	subnet := resp.Subnet
	d.SetId(*subnet.SubnetId)
	log.Printf("[INFO] OLIVIER1 Subnet create vpc: %v Subnet ID: %s", d.Get("vpc_id").(string), *subnet.SubnetId)

	// Wait for the Subnet to become available
	log.Printf("[DEBUG] OLIVIER1 Waiting for subnet (%s) to become available", *subnet.SubnetId)
	stateConf := &resource.StateChangeConf{
		Pending: []string{"pending"},
		Target:  []string{"available"},
		Refresh: SubnetStateRefreshFunc(conn, *subnet.SubnetId),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}

	_, err = stateConf.WaitForState()

	if err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet create vpc: %v Subnet ID: %s Error waiting for subnet to become ready: %s", d.Get("vpc_id").(string), d.Id(), err)
		return fmt.Errorf(
			"Error waiting for subnet (%s) to become ready: %s",
			d.Id(), err)
	}
	log.Printf("[DEBUG] OLIVIER1 Subnet create vpc: %v Subnet ID: %s OK", d.Get("vpc_id").(string), *subnet.SubnetId)

	return resourceAwsSubnetUpdate(d, meta)
}

func resourceAwsSubnetRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] OLIVIER1 Subnet read: %v Timeout: %v", d.Id(), d.Timeout(schema.TimeoutDelete))
	conn := meta.(*AWSClient).ec2conn

	resp, err := conn.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: []*string{aws.String(d.Id())},
	})

	if err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet read: %v Error: %s", d.Id(), err)
		if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidSubnetID.NotFound" {
			log.Printf("[DEBUG] OLIVIER1 Subnet read: %v No longer exist: %s", d.Id(), err)
			// Update state to indicate the subnet no longer exists.
			d.SetId("")
			return nil
		}
		return err
	}
	if resp == nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet read: %v NIL", d.Id())
		return nil
	}

	subnet := resp.Subnets[0]

	d.Set("vpc_id", subnet.VpcId)
	d.Set("availability_zone", subnet.AvailabilityZone)
	d.Set("availability_zone_id", subnet.AvailabilityZoneId)
	d.Set("cidr_block", subnet.CidrBlock)
	d.Set("map_public_ip_on_launch", subnet.MapPublicIpOnLaunch)
	d.Set("assign_ipv6_address_on_creation", subnet.AssignIpv6AddressOnCreation)

	// Make sure those values are set, if an IPv6 block exists it'll be set in the loop
	d.Set("ipv6_cidr_block_association_id", "")
	d.Set("ipv6_cidr_block", "")

	for _, a := range subnet.Ipv6CidrBlockAssociationSet {
		if *a.Ipv6CidrBlockState.State == "associated" { //we can only ever have 1 IPv6 block associated at once
			d.Set("ipv6_cidr_block_association_id", a.AssociationId)
			d.Set("ipv6_cidr_block", a.Ipv6CidrBlock)
			break
		}
	}

	d.Set("arn", subnet.SubnetArn)
	d.Set("tags", tagsToMap(subnet.Tags))
	d.Set("owner_id", subnet.OwnerId)
	log.Printf("[DEBUG] OLIVIER1 Subnet read: %v OK", d.Id())

	return nil
}

func resourceAwsSubnetUpdate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] OLIVIER1 Subnet update: %v Timeout: %v", d.Id(), d.Timeout(schema.TimeoutDelete))
	conn := meta.(*AWSClient).ec2conn

	d.Partial(true)

	if err := setTags(conn, d); err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet update: %v SetTags Error: %s", d.Id(), err)
		return err
	} else {
		d.SetPartial("tags")
	}

	if d.HasChange("map_public_ip_on_launch") {
		modifyOpts := &ec2.ModifySubnetAttributeInput{
			SubnetId: aws.String(d.Id()),
			MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{
				Value: aws.Bool(d.Get("map_public_ip_on_launch").(bool)),
			},
		}

		log.Printf("[DEBUG] Subnet modify attributes: %#v", modifyOpts)

		_, err := conn.ModifySubnetAttribute(modifyOpts)

		if err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet update: %v ModifySubnetAttribute Error: %s", d.Id(), err)
			return err
		} else {
			d.SetPartial("map_public_ip_on_launch")
		}
	}

	// We have to be careful here to not go through a change of association if this is a new resource
	// A New resource here would denote that the Update func is called by the Create func
	if d.HasChange("ipv6_cidr_block") && !d.IsNewResource() {
		// We need to handle that we disassociate the IPv6 CIDR block before we try and associate the new one
		// This could be an issue as, we could error out when we try and add the new one
		// We may need to roll back the state and reattach the old one if this is the case

		_, new := d.GetChange("ipv6_cidr_block")

		if v, ok := d.GetOk("ipv6_cidr_block_association_id"); ok {

			//Firstly we have to disassociate the old IPv6 CIDR Block
			disassociateOps := &ec2.DisassociateSubnetCidrBlockInput{
				AssociationId: aws.String(v.(string)),
			}

			_, err := conn.DisassociateSubnetCidrBlock(disassociateOps)
			if err != nil {
				log.Printf("[DEBUG] OLIVIER1 Subnet update: %v DisassociateSubnetCidrBlock Error: %s", d.Id(), err)
				return err
			}

			// Wait for the CIDR to become disassociated
			log.Printf(
				"[DEBUG] Waiting for IPv6 CIDR (%s) to become disassociated",
				d.Id())
			stateConf := &resource.StateChangeConf{
				Pending: []string{"disassociating", "associated"},
				Target:  []string{"disassociated"},
				Refresh: SubnetIpv6CidrStateRefreshFunc(conn, d.Id(), d.Get("ipv6_cidr_block_association_id").(string)),
				Timeout: 3 * time.Minute,
			}
			if _, err := stateConf.WaitForState(); err != nil {
				log.Printf("[DEBUG] OLIVIER1 Subnet update: %v WaitForState 1 Error: %s", d.Id(), err)
				return fmt.Errorf(
					"Error waiting for IPv6 CIDR (%s) to become disassociated: %s",
					d.Id(), err)
			}
		}

		//Now we need to try and associate the new CIDR block
		associatesOpts := &ec2.AssociateSubnetCidrBlockInput{
			SubnetId:      aws.String(d.Id()),
			Ipv6CidrBlock: aws.String(new.(string)),
		}

		resp, err := conn.AssociateSubnetCidrBlock(associatesOpts)
		if err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet update: %v AssociateSubnetCidrBlock Error: %s", d.Id(), err)
			//The big question here is, do we want to try and reassociate the old one??
			//If we have a failure here, then we may be in a situation that we have nothing associated
			return err
		}

		// Wait for the CIDR to become associated
		log.Printf(
			"[DEBUG] Waiting for IPv6 CIDR (%s) to become associated",
			d.Id())
		stateConf := &resource.StateChangeConf{
			Pending: []string{"associating", "disassociated"},
			Target:  []string{"associated"},
			Refresh: SubnetIpv6CidrStateRefreshFunc(conn, d.Id(), *resp.Ipv6CidrBlockAssociation.AssociationId),
			Timeout: 3 * time.Minute,
		}
		if _, err := stateConf.WaitForState(); err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet update: %v WaitForState 2 Error: %s", d.Id(), err)
			return fmt.Errorf(
				"Error waiting for IPv6 CIDR (%s) to become associated: %s",
				d.Id(), err)
		}

		d.SetPartial("ipv6_cidr_block")
	}

	if d.HasChange("assign_ipv6_address_on_creation") {
		modifyOpts := &ec2.ModifySubnetAttributeInput{
			SubnetId: aws.String(d.Id()),
			AssignIpv6AddressOnCreation: &ec2.AttributeBooleanValue{
				Value: aws.Bool(d.Get("assign_ipv6_address_on_creation").(bool)),
			},
		}

		log.Printf("[DEBUG] Subnet modify attributes: %#v", modifyOpts)

		_, err := conn.ModifySubnetAttribute(modifyOpts)

		if err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet update: %v ModifySubnetAttribute Error: %s", d.Id(), err)
			return err
		} else {
			d.SetPartial("assign_ipv6_address_on_creation")
		}
	}

	d.Partial(false)
	log.Printf("[DEBUG] OLIVIER1 Subnet update: %v OK", d.Id())

	return resourceAwsSubnetRead(d, meta)
}

func resourceAwsSubnetDelete(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v Timeout: %v", d.Id(), d.Timeout(schema.TimeoutDelete))
	conn := meta.(*AWSClient).ec2conn

	log.Printf("[INFO] Deleting subnet: %s", d.Id())

	if err := deleteLingeringLambdaENIs(conn, d, "subnet-id"); err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v Failed to delete Lambda ENIs: %s", d.Id(), err)
		return fmt.Errorf("Failed to delete Lambda ENIs: %s", err)
	}

	req := &ec2.DeleteSubnetInput{
		SubnetId: aws.String(d.Id()),
	}

	locTimeout := d.Timeout(schema.TimeoutDelete)
	if int64(locTimeout) <= int64(20*time.Minute) {
		locTimeout = *schema.DefaultTimeout(34 * time.Minute)
		log.Printf("[DEBUG] OLIVIER1 FIX IN PROGRESS (3) resourceAwsSubnetDelete %s Timeout: %v / %v", d.Id(), locTimeout, d.Timeout(schema.TimeoutDelete))
	}

	wait := resource.StateChangeConf{
		Pending:    []string{"pending"},
		Target:     []string{"destroyed"},
		Timeout:    locTimeout,
		MinTimeout: 1 * time.Second,
		Refresh: func() (interface{}, string, error) {
			_, err := conn.DeleteSubnet(req)
			if err != nil {
				log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: %s", d.Id(), err)
				if apiErr, ok := err.(awserr.Error); ok {
					log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: AWS error %s", d.Id(), err)
					if apiErr.Code() == "DependencyViolation" {
						// There is some pending operation, so just retry
						// in a bit.
						log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: AWS error DependencyViolation %s", d.Id(), err)
						return 42, "pending", nil
					}

					if apiErr.Code() == "InvalidSubnetID.NotFound" {
						log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: AWS error InvalidSubnetID.NotFound %s", d.Id(), err)
						return 42, "destroyed", nil
					}
				}
				log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: AWS error OTHER1 %s", d.Id(), err)

				return 42, "failure", err
			}
			log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v StateChangeRefresh: OK", d.Id())

			return 42, "destroyed", nil
		},
	}

	if _, err := wait.WaitForState(); err != nil {
		log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v Error deleting subnets: %s", d.Id(), err)
		return fmt.Errorf("Error deleting subnet: %s", err)
	}
	log.Printf("[DEBUG] OLIVIER1 Subnet destroy ID: %v OK", d.Id())

	return nil
}

// SubnetStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch a Subnet.
func SubnetStateRefreshFunc(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v", id)
		resp, err := conn.DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: []*string{aws.String(id)},
		})
		if err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v Error %s", id, err)
			if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidSubnetID.NotFound" {
				log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v Error InvalidSubnetID.NotFound %s", id, err)
				resp = nil
			} else {
				log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v Error on SubnetStateRefresh: %s", id, err)
				return nil, "", err
			}
		}

		if resp == nil {
			// Sometimes AWS just has consistency issues and doesn't see
			// our instance yet. Return an empty state.
			log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v NIL", id)
			return nil, "", nil
		}

		subnet := resp.Subnets[0]
		log.Printf("[DEBUG] OLIVIER1 Subnet refresh ID: %v OK State:%s", id, *subnet.State)
		return subnet, *subnet.State, nil
	}
}

func SubnetIpv6CidrStateRefreshFunc(conn *ec2.EC2, id string, associationId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v", id)
		opts := &ec2.DescribeSubnetsInput{
			SubnetIds: []*string{aws.String(id)},
		}
		resp, err := conn.DescribeSubnets(opts)
		if err != nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v Error %s", id, err)
			if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidSubnetID.NotFound" {
				log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v Error InvalidSubnetID.NotFound %s", id, err)
				resp = nil
			} else {
				log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v Error on SubnetIpv6CidrStateRefreshFunc: %s", id, err)
				return nil, "", err
			}
		}

		if resp == nil {
			// Sometimes AWS just has consistency issues and doesn't see
			// our instance yet. Return an empty state.
			log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v NIL 1", id)
			return nil, "", nil
		}

		if resp.Subnets[0].Ipv6CidrBlockAssociationSet == nil {
			log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v NIL 2", id)
			return nil, "", nil
		}

		for _, association := range resp.Subnets[0].Ipv6CidrBlockAssociationSet {
			if *association.AssociationId == associationId {
				log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v Association %s", id, *association.Ipv6CidrBlockState.State)
				return association, *association.Ipv6CidrBlockState.State, nil
			}
		}
		log.Printf("[DEBUG] OLIVIER1 Subnet refresh IPv6 CIDR ID: %v OK NIL", id)

		return nil, "", nil
	}
}
