package containers

import (
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/containerregistry/mgmt/2020-11-01-preview/containerregistry"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/containers/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func dataSourceContainerRegistryToken() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Read: dataSourceContainerRegistryTokenRead,

		Timeouts: &pluginsdk.ResourceTimeout{
			Read: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: validate.ContainerRegistryTokenName,
			},

			"container_registry_name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: validate.ContainerRegistryName,
			},

			"resource_group_name": azure.SchemaResourceGroupNameForDataSource(),

			"scope_map_id": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"enabled": {
				Type:     pluginsdk.TypeBool,
				Computed: true,
			},
		},
	}
}

func dataSourceContainerRegistryTokenRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Containers.TokensClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	resourceGroup := d.Get("resource_group_name").(string)
	containerRegistryName := d.Get("container_registry_name").(string)
	name := d.Get("name").(string)

	resp, err := client.Get(ctx, resourceGroup, containerRegistryName, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			return fmt.Errorf("Container Registry token %q was not found in Resource Group %q", name, resourceGroup)
		}

		return fmt.Errorf("Error making Read request on token %q (Azure Container Registry %q, Resource Group %q): %+v", name, containerRegistryName, resourceGroup, err)
	}

	if resp.ID == nil || *resp.ID == "" {
		return fmt.Errorf("retrieving Container Registry Token %q (Azure Container Registry %q, Resource Group %q): `id` was nil", name, containerRegistryName, resourceGroup)
	}

	status := true
	if resp.Status == containerregistry.TokenStatusDisabled {
		status = false
	}

	d.SetId(*resp.ID)
	d.Set("name", resp.Name)
	d.Set("resource_group_name", resourceGroup)
	d.Set("container_registry_name", containerRegistryName)
	d.Set("scope_map_id", resp.ScopeMapID)
	d.Set("enabled", status)

	return nil
}
