package iothub

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/iothub/mgmt/2020-03-01/devices"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/locks"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/iothub/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceIotHubEndpointServiceBusQueue() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceIotHubEndpointServiceBusQueueCreateUpdate,
		Read:   resourceIotHubEndpointServiceBusQueueRead,
		Update: resourceIotHubEndpointServiceBusQueueCreateUpdate,
		Delete: resourceIotHubEndpointServiceBusQueueDelete,
		// TODO: replace this with an importer which validates the ID during import
		Importer: pluginsdk.DefaultImporter(),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.IoTHubEndpointName,
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"iothub_name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.IoTHubName,
			},

			"connection_string": {
				Type:     pluginsdk.TypeString,
				Required: true,
				DiffSuppressFunc: func(k, old, new string, d *pluginsdk.ResourceData) bool {
					sharedAccessKeyRegex := regexp.MustCompile("SharedAccessKey=[^;]+")
					sbProtocolRegex := regexp.MustCompile("sb://([^:]+)(:5671)?/;")

					maskedNew := sbProtocolRegex.ReplaceAllString(new, "sb://$1:5671/;")
					maskedNew = sharedAccessKeyRegex.ReplaceAllString(maskedNew, "SharedAccessKey=****")
					return (new == d.Get(k).(string)) && (maskedNew == old)
				},
				Sensitive: true,
			},
		},
	}
}

func resourceIotHubEndpointServiceBusQueueCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).IoTHub.ResourceClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()
	subscriptionID := meta.(*clients.Client).Account.SubscriptionId

	iothubName := d.Get("iothub_name").(string)
	resourceGroup := d.Get("resource_group_name").(string)

	locks.ByName(iothubName, IothubResourceName)
	defer locks.UnlockByName(iothubName, IothubResourceName)

	iothub, err := client.Get(ctx, resourceGroup, iothubName)
	if err != nil {
		if utils.ResponseWasNotFound(iothub.Response) {
			return fmt.Errorf("IotHub %q (Resource Group %q) was not found", iothubName, resourceGroup)
		}

		return fmt.Errorf("Error loading IotHub %q (Resource Group %q): %+v", iothubName, resourceGroup, err)
	}

	endpointName := d.Get("name").(string)
	resourceId := fmt.Sprintf("%s/Endpoints/%s", *iothub.ID, endpointName)

	queueEndpoint := devices.RoutingServiceBusQueueEndpointProperties{
		ConnectionString: utils.String(d.Get("connection_string").(string)),
		Name:             utils.String(endpointName),
		SubscriptionID:   utils.String(subscriptionID),
		ResourceGroup:    utils.String(resourceGroup),
	}

	routing := iothub.Properties.Routing
	if routing == nil {
		routing = &devices.RoutingProperties{}
	}

	if routing.Endpoints == nil {
		routing.Endpoints = &devices.RoutingEndpoints{}
	}

	if routing.Endpoints.EventHubs == nil {
		queues := make([]devices.RoutingServiceBusQueueEndpointProperties, 0)
		routing.Endpoints.ServiceBusQueues = &queues
	}
	endpoints := make([]devices.RoutingServiceBusQueueEndpointProperties, 0)

	alreadyExists := false
	for _, existingEndpoint := range *routing.Endpoints.ServiceBusQueues {
		if existingEndpointName := existingEndpoint.Name; existingEndpointName != nil {
			if strings.EqualFold(*existingEndpointName, endpointName) {
				if d.IsNewResource() {
					return tf.ImportAsExistsError("azurerm_iothub_endpoint_servicebus_queue", resourceId)
				}
				endpoints = append(endpoints, queueEndpoint)
				alreadyExists = true
			} else {
				endpoints = append(endpoints, existingEndpoint)
			}
		}
	}

	if d.IsNewResource() {
		endpoints = append(endpoints, queueEndpoint)
	} else if !alreadyExists {
		return fmt.Errorf("Unable to find ServiceBus Queue Endpoint %q defined for IotHub %q (Resource Group %q)", endpointName, iothubName, resourceGroup)
	}
	routing.Endpoints.ServiceBusQueues = &endpoints

	future, err := client.CreateOrUpdate(ctx, resourceGroup, iothubName, iothub, "")
	if err != nil {
		return fmt.Errorf("Error creating/updating IotHub %q (Resource Group %q): %+v", iothubName, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for the completion of the creating/updating of IotHub %q (Resource Group %q): %+v", iothubName, resourceGroup, err)
	}

	d.SetId(resourceId)

	return resourceIotHubEndpointServiceBusQueueRead(d, meta)
}

func resourceIotHubEndpointServiceBusQueueRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).IoTHub.ResourceClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	parsedIothubEndpointId, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := parsedIothubEndpointId.ResourceGroup
	iothubName := parsedIothubEndpointId.Path["IotHubs"]
	endpointName := parsedIothubEndpointId.Path["Endpoints"]

	iothub, err := client.Get(ctx, resourceGroup, iothubName)
	if err != nil {
		return fmt.Errorf("Error loading IotHub %q (Resource Group %q): %+v", iothubName, resourceGroup, err)
	}

	d.Set("name", endpointName)
	d.Set("iothub_name", iothubName)
	d.Set("resource_group_name", resourceGroup)

	if iothub.Properties == nil || iothub.Properties.Routing == nil || iothub.Properties.Routing.Endpoints == nil {
		return nil
	}

	if endpoints := iothub.Properties.Routing.Endpoints.ServiceBusQueues; endpoints != nil {
		for _, endpoint := range *endpoints {
			if existingEndpointName := endpoint.Name; existingEndpointName != nil {
				if strings.EqualFold(*existingEndpointName, endpointName) {
					d.Set("connection_string", endpoint.ConnectionString)
				}
			}
		}
	}

	return nil
}

func resourceIotHubEndpointServiceBusQueueDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).IoTHub.ResourceClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	parsedIothubEndpointId, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := parsedIothubEndpointId.ResourceGroup
	iothubName := parsedIothubEndpointId.Path["IotHubs"]
	endpointName := parsedIothubEndpointId.Path["Endpoints"]

	locks.ByName(iothubName, IothubResourceName)
	defer locks.UnlockByName(iothubName, IothubResourceName)

	iothub, err := client.Get(ctx, resourceGroup, iothubName)
	if err != nil {
		if utils.ResponseWasNotFound(iothub.Response) {
			return fmt.Errorf("IotHub %q (Resource Group %q) was not found", iothubName, resourceGroup)
		}

		return fmt.Errorf("Error loading IotHub %q (Resource Group %q): %+v", iothubName, resourceGroup, err)
	}

	if iothub.Properties == nil || iothub.Properties.Routing == nil || iothub.Properties.Routing.Endpoints == nil {
		return nil
	}
	endpoints := iothub.Properties.Routing.Endpoints.ServiceBusQueues

	if endpoints == nil {
		return nil
	}

	updatedEndpoints := make([]devices.RoutingServiceBusQueueEndpointProperties, 0)
	for _, endpoint := range *endpoints {
		if existingEndpointName := endpoint.Name; existingEndpointName != nil {
			if !strings.EqualFold(*existingEndpointName, endpointName) {
				updatedEndpoints = append(updatedEndpoints, endpoint)
			}
		}
	}

	iothub.Properties.Routing.Endpoints.ServiceBusQueues = &updatedEndpoints

	future, err := client.CreateOrUpdate(ctx, resourceGroup, iothubName, iothub, "")
	if err != nil {
		return fmt.Errorf("Error updating IotHub %q (Resource Group %q) with ServiceBus Queue Endpoint %q: %+v", iothubName, resourceGroup, endpointName, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for IotHub %q (Resource Group %q) to finish updating ServiceBus Queue Endpoint %q: %+v", iothubName, resourceGroup, endpointName, err)
	}

	return nil
}
