# Changelog

## 2.0.0 (unreleased)

### BREAKING CHANGES

- Provider attribute `api_url` renamed to `ipam_api_url`.
- Provider attribute `scope` replaced by `ipam_application_id`. The OAuth scope (`api://<id>/.default`) is now constructed automatically — only the Azure AD application (client) ID is required.
- Environment variable `AZUREIPAM_SCOPE` replaced by `AZUREIPAM_APPLICATION_ID`.

### Features

- Concurrent reservation creates and deletes are now serialized via an internal write mutex, preventing `500`/`403` errors caused by IPAM API concurrency conflicts.
- Retry logic added to `CreateReservation` and `DeleteReservation` with exponential backoff and jitter (up to 3 retries) for transient `500` and `403` responses.

### Improvements

- HTTP client timeout reduced from 30s to 10s.
- Provider documentation and examples updated to reflect renamed attributes.
