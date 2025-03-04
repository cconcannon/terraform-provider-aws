package rds

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/flex"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
)

func DataSourceClusterSnapshot() *schema.Resource {
	return &schema.Resource{
		ReadWithoutTimeout: dataSourceClusterSnapshotRead,

		Schema: map[string]*schema.Schema{
			//selection criteria
			"db_cluster_identifier": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"db_cluster_snapshot_identifier": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"snapshot_type": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"include_shared": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"include_public": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"most_recent": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			//Computed values returned
			"allocated_storage": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"availability_zones": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
			},
			"db_cluster_snapshot_arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"storage_encrypted": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"engine": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"engine_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"kms_key_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"license_model": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"port": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"source_db_cluster_snapshot_arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"snapshot_create_time": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"vpc_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags": tftags.TagsSchemaComputed(),
		},
	}
}

func dataSourceClusterSnapshotRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).RDSConn()
	ignoreTagsConfig := meta.(*conns.AWSClient).IgnoreTagsConfig

	clusterIdentifier, clusterIdentifierOk := d.GetOk("db_cluster_identifier")
	snapshotIdentifier, snapshotIdentifierOk := d.GetOk("db_cluster_snapshot_identifier")

	if !clusterIdentifierOk && !snapshotIdentifierOk {
		return sdkdiag.AppendErrorf(diags, "One of db_cluster_snapshot_identifier or db_cluster_identifier must be assigned")
	}

	params := &rds.DescribeDBClusterSnapshotsInput{
		IncludePublic: aws.Bool(d.Get("include_public").(bool)),
		IncludeShared: aws.Bool(d.Get("include_shared").(bool)),
	}
	if v, ok := d.GetOk("snapshot_type"); ok {
		params.SnapshotType = aws.String(v.(string))
	}
	if clusterIdentifierOk {
		params.DBClusterIdentifier = aws.String(clusterIdentifier.(string))
	}
	if snapshotIdentifierOk {
		params.DBClusterSnapshotIdentifier = aws.String(snapshotIdentifier.(string))
	}

	log.Printf("[DEBUG] Reading DB Cluster Snapshot: %s", params)
	resp, err := conn.DescribeDBClusterSnapshotsWithContext(ctx, params)
	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading RDS Cluster Snapshot (%s): %s", d.Id(), err)
	}

	if len(resp.DBClusterSnapshots) < 1 {
		return sdkdiag.AppendErrorf(diags, "Your query returned no results. Please change your search criteria and try again.")
	}

	var snapshot *rds.DBClusterSnapshot
	if len(resp.DBClusterSnapshots) > 1 {
		recent := d.Get("most_recent").(bool)
		log.Printf("[DEBUG] aws_db_cluster_snapshot - multiple results found and `most_recent` is set to: %t", recent)
		if recent {
			snapshot = mostRecentClusterSnapshot(resp.DBClusterSnapshots)
		} else {
			return sdkdiag.AppendErrorf(diags, "Your query returned more than one result. Please try a more specific search criteria.")
		}
	} else {
		snapshot = resp.DBClusterSnapshots[0]
	}

	d.SetId(aws.StringValue(snapshot.DBClusterSnapshotIdentifier))
	d.Set("allocated_storage", snapshot.AllocatedStorage)
	if err := d.Set("availability_zones", flex.FlattenStringList(snapshot.AvailabilityZones)); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting availability_zones: %s", err)
	}
	d.Set("db_cluster_identifier", snapshot.DBClusterIdentifier)
	d.Set("db_cluster_snapshot_arn", snapshot.DBClusterSnapshotArn)
	d.Set("db_cluster_snapshot_identifier", snapshot.DBClusterSnapshotIdentifier)
	d.Set("engine", snapshot.Engine)
	d.Set("engine_version", snapshot.EngineVersion)
	d.Set("kms_key_id", snapshot.KmsKeyId)
	d.Set("license_model", snapshot.LicenseModel)
	d.Set("port", snapshot.Port)
	if snapshot.SnapshotCreateTime != nil {
		d.Set("snapshot_create_time", snapshot.SnapshotCreateTime.Format(time.RFC3339))
	}
	d.Set("snapshot_type", snapshot.SnapshotType)
	d.Set("source_db_cluster_snapshot_arn", snapshot.SourceDBClusterSnapshotArn)
	d.Set("status", snapshot.Status)
	d.Set("storage_encrypted", snapshot.StorageEncrypted)
	d.Set("vpc_id", snapshot.VpcId)

	tags, err := ListTags(ctx, conn, d.Get("db_cluster_snapshot_arn").(string))

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "listing tags for RDS DB Cluster Snapshot (%s): %s", d.Get("db_cluster_snapshot_arn").(string), err)
	}

	if err := d.Set("tags", tags.IgnoreAWS().IgnoreConfig(ignoreTagsConfig).Map()); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting tags: %s", err)
	}

	return diags
}

type rdsClusterSnapshotSort []*rds.DBClusterSnapshot

func (a rdsClusterSnapshotSort) Len() int      { return len(a) }
func (a rdsClusterSnapshotSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a rdsClusterSnapshotSort) Less(i, j int) bool {
	// Snapshot creation can be in progress
	if a[i].SnapshotCreateTime == nil {
		return true
	}
	if a[j].SnapshotCreateTime == nil {
		return false
	}

	return (*a[i].SnapshotCreateTime).Before(*a[j].SnapshotCreateTime)
}

func mostRecentClusterSnapshot(snapshots []*rds.DBClusterSnapshot) *rds.DBClusterSnapshot {
	sortedSnapshots := snapshots
	sort.Sort(rdsClusterSnapshotSort(sortedSnapshots))
	return sortedSnapshots[len(sortedSnapshots)-1]
}
