package dns

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2020-09-01/dns/mgmt/dns"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonschema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-azurestack/internal/az/tags"
	"github.com/hashicorp/terraform-provider-azurestack/internal/clients"
	"github.com/hashicorp/terraform-provider-azurestack/internal/services/dns/parse"
	"github.com/hashicorp/terraform-provider-azurestack/internal/tf"
	"github.com/hashicorp/terraform-provider-azurestack/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurestack/internal/tf/timeouts"
	"github.com/hashicorp/terraform-provider-azurestack/internal/utils"
)

func dnsTxtRecord() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: dnsTxtRecordCreateUpdate,
		Read:   dnsTxtRecordRead,
		Update: dnsTxtRecordCreateUpdate,
		Delete: dnsTxtRecordDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},
		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.TxtRecordID(id)
			return err
		}),
		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
			},

			"resource_group_name": commonschema.ResourceGroupName(),

			"zone_name": {
				Type:     pluginsdk.TypeString,
				Required: true,
			},

			"record": {
				Type:     pluginsdk.TypeSet,
				Required: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"value": {
							Type:         pluginsdk.TypeString,
							Required:     true,
							ValidateFunc: validation.StringLenBetween(1, 1024),
						},
					},
				},
			},

			"ttl": {
				Type:     pluginsdk.TypeInt,
				Required: true,
			},

			"fqdn": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"tags": tags.Schema(),
		},
	}
}

func dnsTxtRecordCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Dns.RecordSetsClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	defer cancel()

	name := d.Get("name").(string)
	resGroup := d.Get("resource_group_name").(string)
	zoneName := d.Get("zone_name").(string)

	resourceId := parse.NewTxtRecordID(subscriptionId, resGroup, zoneName, name)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resGroup, zoneName, name, dns.TXT)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing DNS TXT Record %q (Zone %q / Resource Group %q): %s", name, zoneName, resGroup, err)
			}
		}

		if !utils.ResponseWasNotFound(existing.Response) {
			return tf.ImportAsExistsError("azurestack_dns_txt_record", resourceId.ID())
		}
	}

	ttl := int64(d.Get("ttl").(int))
	t := d.Get("tags").(map[string]interface{})

	parameters := dns.RecordSet{
		Name: &name,
		RecordSetProperties: &dns.RecordSetProperties{
			Metadata:   tags.Expand(t),
			TTL:        &ttl,
			TxtRecords: expandazurestackDnsTxtRecords(d),
		},
	}

	eTag := ""
	ifNoneMatch := "" // set to empty to allow updates to records after creation
	if _, err := client.CreateOrUpdate(ctx, resGroup, zoneName, name, dns.TXT, parameters, eTag, ifNoneMatch); err != nil {
		return fmt.Errorf("creating/updating DNS TXT Record %q (Zone %q / Resource Group %q): %s", name, zoneName, resGroup, err)
	}

	d.SetId(resourceId.ID())

	return dnsTxtRecordRead(d, meta)
}

func dnsTxtRecordRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Dns.RecordSetsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.TxtRecordID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.DnszoneName, id.TXTName, dns.TXT)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("reading DNS TXT record %s: %+v", id.TXTName, err)
	}

	d.Set("name", id.TXTName)
	d.Set("resource_group_name", id.ResourceGroup)
	d.Set("zone_name", id.DnszoneName)
	d.Set("ttl", resp.TTL)
	d.Set("fqdn", resp.Fqdn)

	if err := d.Set("record", flattenazurestackDnsTxtRecords(resp.TxtRecords)); err != nil {
		return err
	}
	return tags.FlattenAndSet(d, resp.Metadata)
}

func dnsTxtRecordDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Dns.RecordSetsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.TxtRecordID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Delete(ctx, id.ResourceGroup, id.DnszoneName, id.TXTName, dns.TXT, "")
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deleting DNS TXT Record %s: %+v", id.TXTName, err)
	}

	return nil
}

func flattenazurestackDnsTxtRecords(records *[]dns.TxtRecord) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if records != nil {
		for _, record := range *records {
			txtRecord := make(map[string]interface{})

			if v := record.Value; v != nil {
				value := strings.Join(*v, "")
				txtRecord["value"] = value
			}

			results = append(results, txtRecord)
		}
	}

	return results
}

func expandazurestackDnsTxtRecords(d *pluginsdk.ResourceData) *[]dns.TxtRecord {
	recordStrings := d.Get("record").(*pluginsdk.Set).List()
	records := make([]dns.TxtRecord, len(recordStrings))

	segmentLen := 254
	for i, v := range recordStrings {
		record := v.(map[string]interface{})
		v := record["value"].(string)

		var value []string
		for len(v) > segmentLen {
			value = append(value, v[:segmentLen])
			v = v[segmentLen:]
		}
		value = append(value, v)

		txtRecord := dns.TxtRecord{
			Value: &value,
		}

		records[i] = txtRecord
	}

	return &records
}
