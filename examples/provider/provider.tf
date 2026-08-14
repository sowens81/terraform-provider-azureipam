# We strongly recommend using the required_providers block to set the
# azureipam provider source and version being used
terraform {
  required_providers {
    azureipam = {
      version = "~>2.0"
      source  = "sowens81/azureipam"
    }
  }
}

# Configure the Azure IPAM provider
provider "azureipam" {
  ipam_api_url           = "https://myazureipam.azurewebsites.net"
  ipam_application_id    = "xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxx" #ApplicationId of the Engine Azure AD Application, see also the [IPAM deployment documentation](https://github.com/Azure/ipam/tree/main/docs/deployment)
  skip_cert_verification = true                               //ONLY recommended for development environments
}
