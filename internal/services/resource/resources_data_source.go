package resource

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2020-09-01/resources/mgmt/resources"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonschema"
	"github.com/hashicorp/go-uuid"

	"github.com/hashicorp/terraform-provider-azurestack/internal/az/tags"
	"github.com/hashicorp/terraform-provider-azurestack/internal/clients"
	"github.com/hashicorp/terraform-provider-azurestack/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurestack/internal/tf/timeouts"
)

func resourcesDataSource() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Read: resourcesDataSourceRead,
		Timeouts: &pluginsdk.ResourceTimeout{
			Read: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
			},
			"resource_group_name": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
			},
			"type": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
			},

			"required_tags": tags.Schema(),

			"resources": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"name": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},

						"id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},

						"type": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},

						"location": commonschema.Location(),

						"tags": tags.SchemaDataSource(),
					},
				},
			},
		},
	}
}

func resourcesDataSourceRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Resource.ResourcesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	resourceGroupName := d.Get("resource_group_name").(string)
	resourceName := d.Get("name").(string)
	resourceType := d.Get("type").(string)
	requiredTags := d.Get("required_tags").(map[string]interface{})

	if resourceGroupName == "" && resourceName == "" && resourceType == "" {
		return fmt.Errorf("At least one of `name`, `resource_group_name` or `type` must be specified")
	}

	var filter string

	if resourceGroupName != "" {
		v := fmt.Sprintf("resourceGroup eq '%s'", resourceGroupName)
		filter += v
	}

	if resourceName != "" {
		if strings.Contains(filter, "eq") {
			filter += " and "
		}
		v := fmt.Sprintf("name eq '%s'", resourceName)
		filter += v
	}

	if resourceType != "" {
		if strings.Contains(filter, "eq") {
			filter += " and "
		}
		v := fmt.Sprintf("resourceType eq '%s'", resourceType)
		filter += v
	}

	// Use List instead of listComplete because of bug in SDK: https://github.com/Azure/azure-sdk-for-go/issues/9510
	resources := make([]map[string]interface{}, 0)
	resourcesResp, err := client.List(ctx, filter, "", nil)
	if err != nil {
		return fmt.Errorf("getting resources: %+v", err)
	}

	resources = append(resources, filterResource(resourcesResp.Values(), requiredTags)...)
	for resourcesResp.Response().NextLink != nil && *resourcesResp.Response().NextLink != "" {
		if err := resourcesResp.NextWithContext(ctx); err != nil {
			return fmt.Errorf("loading Resource List: %+v", err)
		}
		resources = append(resources, filterResource(resourcesResp.Values(), requiredTags)...)
	}

	uuid, _ := uuid.GenerateUUID()
	d.SetId("resource-" + uuid)
	if err := d.Set("resources", resources); err != nil {
		return fmt.Errorf("setting `resources`: %+v", err)
	}
	return nil
}

func filterResource(inputs []resources.GenericResourceExpanded, requiredTags map[string]interface{}) []map[string]interface{} {
	var result []map[string]interface{}
	for _, res := range inputs {
		if res.ID == nil {
			continue
		}

		// currently its not supported to use tags filter with other filters
		// therefore we need to filter the resources manually.
		tagMatches := 0
		if res.Tags != nil {
			for requiredTagName, requiredTagVal := range requiredTags {
				for tagName, tagVal := range res.Tags {
					if requiredTagName == tagName && tagVal != nil && requiredTagVal == *tagVal {
						tagMatches++
					}
				}
			}
		}

		if tagMatches == len(requiredTags) {
			resName := ""
			if res.Name != nil {
				resName = *res.Name
			}

			resID := ""
			if res.ID != nil {
				resID = *res.ID
			}

			resType := ""
			if res.Type != nil {
				resType = *res.Type
			}

			resLocation := ""
			if res.Location != nil {
				resLocation = *res.Location
			}

			resTags := make(map[string]interface{})
			if res.Tags != nil {
				resTags = make(map[string]interface{}, len(res.Tags))
				for key, value := range res.Tags {
					if value != nil {
						resTags[key] = *value
					}
				}
			}

			result = append(result, map[string]interface{}{
				"name":     resName,
				"id":       resID,
				"type":     resType,
				"location": resLocation,
				"tags":     resTags,
			})
		} else {
			log.Printf("[DEBUG] azurestack_resources - resources %q (id: %q) skipped as a required tag is not set or has the wrong value.", *res.Name, *res.ID)
		}
	}
	return result
}
