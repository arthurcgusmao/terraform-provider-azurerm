package keyvault

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.1/keyvault"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/keyvault/parse"
	keyVaultValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/keyvault/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func dataSourceKeyVaultCertificate() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Read: dataSourceKeyVaultCertificateRead,

		Timeouts: &pluginsdk.ResourceTimeout{
			Read: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: keyVaultValidate.NestedItemName,
			},

			"key_vault_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: keyVaultValidate.VaultID,
			},

			"version": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
			},

			// Computed
			"certificate_policy": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"issuer_parameters": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"name": {
										Type:     pluginsdk.TypeString,
										Computed: true,
									},
								},
							},
						},
						"key_properties": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"curve": {
										Type:     pluginsdk.TypeString,
										Computed: true,
									},
									"exportable": {
										Type:     pluginsdk.TypeBool,
										Computed: true,
									},
									"key_size": {
										Type:     pluginsdk.TypeInt,
										Computed: true,
									},
									"key_type": {
										Type:     pluginsdk.TypeString,
										Computed: true,
									},
									"reuse_key": {
										Type:     pluginsdk.TypeBool,
										Computed: true,
									},
								},
							},
						},

						"lifetime_action": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"action": {
										Type:     pluginsdk.TypeList,
										Computed: true,
										Elem: &pluginsdk.Resource{
											Schema: map[string]*pluginsdk.Schema{
												"action_type": {
													Type:     pluginsdk.TypeString,
													Computed: true,
												},
											},
										},
									},
									"trigger": {
										Type:     pluginsdk.TypeList,
										Computed: true,
										Elem: &pluginsdk.Resource{
											Schema: map[string]*pluginsdk.Schema{
												"days_before_expiry": {
													Type:     pluginsdk.TypeInt,
													Computed: true,
												},
												"lifetime_percentage": {
													Type:     pluginsdk.TypeInt,
													Computed: true,
												},
											},
										},
									},
								},
							},
						},

						"secret_properties": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"content_type": {
										Type:     pluginsdk.TypeString,
										Computed: true,
									},
								},
							},
						},

						"x509_certificate_properties": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"extended_key_usage": {
										Type:     pluginsdk.TypeList,
										Computed: true,
										Elem: &pluginsdk.Schema{
											Type: pluginsdk.TypeString,
										},
									},
									"key_usage": {
										Type:     pluginsdk.TypeList,
										Computed: true,
										Elem: &pluginsdk.Schema{
											Type: pluginsdk.TypeString,
										},
									},
									"subject": {
										Type:     pluginsdk.TypeString,
										Computed: true,
									},
									"subject_alternative_names": {
										Type:     pluginsdk.TypeList,
										Computed: true,
										Elem: &pluginsdk.Resource{
											Schema: map[string]*pluginsdk.Schema{
												"emails": {
													Type:     pluginsdk.TypeList,
													Computed: true,
													Elem: &pluginsdk.Schema{
														Type: pluginsdk.TypeString,
													},
												},
												"dns_names": {
													Type:     pluginsdk.TypeList,
													Computed: true,
													Elem: &pluginsdk.Schema{
														Type: pluginsdk.TypeString,
													},
												},
												"upns": {
													Type:     pluginsdk.TypeList,
													Computed: true,
													Elem: &pluginsdk.Schema{
														Type: pluginsdk.TypeString,
													},
												},
											},
										},
									},
									"validity_in_months": {
										Type:     pluginsdk.TypeInt,
										Computed: true,
									},
								},
							},
						},
					},
				},
			},

			"secret_id": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"certificate_data": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"certificate_data_base64": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"thumbprint": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"tags": tags.SchemaDataSource(),
		},
	}
}

func dataSourceKeyVaultCertificateRead(d *pluginsdk.ResourceData, meta interface{}) error {
	keyVaultsClient := meta.(*clients.Client).KeyVault
	client := meta.(*clients.Client).KeyVault.ManagementClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	keyVaultId, err := parse.VaultID(d.Get("key_vault_id").(string))
	if err != nil {
		return err
	}
	version := d.Get("version").(string)

	keyVaultBaseUri, err := keyVaultsClient.BaseUriForKeyVault(ctx, *keyVaultId)
	if err != nil {
		return fmt.Errorf("looking up base uri for Key %q in %s: %+v", name, *keyVaultId, err)
	}

	cert, err := client.GetCertificate(ctx, *keyVaultBaseUri, name, version)
	if err != nil {
		if utils.ResponseWasNotFound(cert.Response) {
			log.Printf("[DEBUG] Certificate %q was not found in Key Vault at URI %q - removing from state", name, *keyVaultBaseUri)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error reading Key Vault Certificate: %+v", err)
	}

	if cert.ID == nil || *cert.ID == "" {
		return fmt.Errorf("failure reading Key Vault Certificate ID for %q", name)
	}

	d.SetId(*cert.ID)

	id, err := parse.ParseNestedItemID(*cert.ID)
	if err != nil {
		return err
	}

	d.Set("name", id.Name)

	certificatePolicy := flattenKeyVaultCertificatePolicyForDataSource(cert.Policy)
	if err := d.Set("certificate_policy", certificatePolicy); err != nil {
		return fmt.Errorf("Error setting Key Vault Certificate Policy: %+v", err)
	}

	d.Set("version", id.Version)
	d.Set("secret_id", cert.Sid)

	certificateData := ""
	if contents := cert.Cer; contents != nil {
		certificateData = strings.ToUpper(hex.EncodeToString(*contents))
	}
	d.Set("certificate_data", certificateData)

	certificateDataBase64 := ""
	if contents := cert.Cer; contents != nil {
		certificateDataBase64 = base64.StdEncoding.EncodeToString(*contents)
	}
	d.Set("certificate_data_base64", certificateDataBase64)

	thumbprint := ""
	if v := cert.X509Thumbprint; v != nil {
		x509Thumbprint, err := base64.RawURLEncoding.DecodeString(*v)
		if err != nil {
			return err
		}

		thumbprint = strings.ToUpper(hex.EncodeToString(x509Thumbprint))
	}
	d.Set("thumbprint", thumbprint)

	return tags.FlattenAndSet(d, cert.Tags)
}

func flattenKeyVaultCertificatePolicyForDataSource(input *keyvault.CertificatePolicy) []interface{} {
	if input == nil {
		return []interface{}{}
	}

	policy := make(map[string]interface{})

	if params := input.IssuerParameters; params != nil {
		var name string
		if params.Name != nil {
			name = *params.Name
		}
		policy["issuer_parameters"] = []interface{}{
			map[string]interface{}{
				"name": name,
			},
		}
	}

	// key properties
	if props := input.KeyProperties; props != nil {
		var curve, keyType string
		var exportable, reuseKey bool
		var keySize int
		curve = string(props.Curve)
		if props.Exportable != nil {
			exportable = *props.Exportable
		}
		if props.ReuseKey != nil {
			reuseKey = *props.ReuseKey
		}
		if props.KeySize != nil {
			keySize = int(*props.KeySize)
		}
		if props.KeyType != "" {
			keyType = string(props.KeyType)
		}

		policy["key_properties"] = []interface{}{
			map[string]interface{}{
				"curve":      curve,
				"exportable": exportable,
				"key_size":   keySize,
				"key_type":   keyType,
				"reuse_key":  reuseKey,
			},
		}
	}

	// lifetime actions
	lifetimeActions := make([]interface{}, 0)
	if actions := input.LifetimeActions; actions != nil {
		for _, action := range *actions {
			lifetimeAction := make(map[string]interface{})

			actionOutput := make(map[string]interface{})
			if act := action.Action; act != nil {
				actionOutput["action_type"] = string(act.ActionType)
			}
			lifetimeAction["action"] = []interface{}{actionOutput}

			triggerOutput := make(map[string]interface{})
			if trigger := action.Trigger; trigger != nil {
				if days := trigger.DaysBeforeExpiry; days != nil {
					triggerOutput["days_before_expiry"] = int(*trigger.DaysBeforeExpiry)
				}

				if days := trigger.LifetimePercentage; days != nil {
					triggerOutput["lifetime_percentage"] = int(*trigger.LifetimePercentage)
				}
			}
			lifetimeAction["trigger"] = []interface{}{triggerOutput}
			lifetimeActions = append(lifetimeActions, lifetimeAction)
		}
	}
	policy["lifetime_action"] = lifetimeActions

	// secret properties
	if props := input.SecretProperties; props != nil {
		var contentType string
		if props.ContentType != nil {
			contentType = *props.ContentType
		}
		policy["secret_properties"] = []interface{}{
			map[string]interface{}{
				"content_type": contentType,
			},
		}
	}

	// x509 Certificate Properties
	if props := input.X509CertificateProperties; props != nil {
		var subject string
		var validityInMonths int
		if props.Subject != nil {
			subject = *props.Subject
		}
		if props.ValidityInMonths != nil {
			validityInMonths = int(*props.ValidityInMonths)
		}

		usages := make([]string, 0)
		if props.KeyUsage != nil {
			for _, usage := range *props.KeyUsage {
				usages = append(usages, string(usage))
			}
		}

		sanOutputs := make([]interface{}, 0)
		if san := props.SubjectAlternativeNames; san != nil {
			sanOutputs = append(sanOutputs, map[string]interface{}{
				"emails":    utils.FlattenStringSlice(san.Emails),
				"dns_names": utils.FlattenStringSlice(san.DNSNames),
				"upns":      utils.FlattenStringSlice(san.Upns),
			})
		}

		policy["x509_certificate_properties"] = []interface{}{
			map[string]interface{}{
				"key_usage":                 usages,
				"subject":                   subject,
				"validity_in_months":        validityInMonths,
				"extended_key_usage":        utils.FlattenStringSlice(props.Ekus),
				"subject_alternative_names": sanOutputs,
			},
		}
	}

	return []interface{}{policy}
}
