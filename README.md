# Vulcan Crontinuous

A cron based scheduler to execute Vulcan scans and digest report generation.

To run execute:

```sh
go build cmd/vulcan-crontinuous
./cmd/vulcan-crontinuous/vulcan-crontinuous -c _resources/config/local.toml
```

## Exposed API

The exposed API is very simple.
It exposes two group of endpoints to handle schedules for scans and reports.

### Scan scheduling

* **Get a snapshot of the current scheduled cron jobs**.

    ```GET ``` to ``` /entries ```

    The endpoint will return a response like this.
```json
 [
    {
        "program_id": "44a57d24-2a23-41a0-a986-2f11a68e9e8b",
        "team_id":"461a62aa-6e1c-11e8-802e-4c32758b498f",
        "cron_spec":"15 * * * *"
    },
    {
        "program_id": "8491b4c9-efd1-4ea0-bd83-a627edb61b65",
        "id":"561a62aa-6e1c-11e8-802e-4c32758b498f",
        "cron_spec":"15 * * * *"
    }
]
```

* **Get a snapshot of the current scheduled cron jobs for a program**.

    ```GET ``` to ``` /entries/:programID ```

    The endpoint will return a response like this.

```json
{
    "program_id": "44a57d24-2a23-41a0-a986-2f11a68e9e8b",
    "team_id":"461a62aa-6e1c-11e8-802e-4c32758b498f",
    "cron_spec":"15 * * * *"
}
```

* **Create or update a cron job**.

    ```POST``` to ``` /settings/:programID/:teamID ``` with a json payload in the body like this:

```json
 {
     "str" : "* * * * * *"
 }
```
    This will create a new cron job that will schedule a scan associated with the given program ID.

    If the program ID already exists it will replace the schedule with the new passed cron string.

* **Bulk set**.

  ```POST``` to ``` /entries/``` with a json payload in the body like this:

```
 [
     {
      "str" : "* * * * * *",
      "program_id":"global_default",
      "team_id":"a_team_id"
      "overwrite": true/false
     },
     {
      "str" : "* * * * * *",
      "program_id":"global_default",
      "team_id":"a_team_id"
      "overwrite": true/false
     }
 ]
```
    This will create a new cron job for each item defined in the array only
    if no other schedule for the same program exists, unless the 'overwrite' param
    is set to true (default if omitted in the payload is false), in that case the
    existent job is overwritten

* **Delete a schedule**.

    ```DELETE``` to: ``` /entries/:programID ``` .

    The end point will return 200 if the entry was deleted and 400 if the entry was not found.

### Report scheduling

* **Get a snapshot of the current scheduled report cron jobs**.

    ```GET ``` to ``` /report/entries ```

    The endpoint will return a response like this.
```
 [
    {
        "team_id":"461a62aa-6e1c-11e8-802e-4c32758b498f",
        "cron_spec":"15 * * * *"
    },
    {
        "id":"561a62aa-6e1c-11e8-802e-4c32758b498f",
        "cron_spec":"15 * * * *"
    }
]
```

* **Get a snapshot of the current scheduled report cron jobs for a team**.

    ```GET ``` to ``` /report/entries/:teamID ```

    The endpoint will return a response like this.
```
{
    "team_id":"461a62aa-6e1c-11e8-802e-4c32758b498f",
    "cron_spec":"15 * * * *"
}
```

* **Create or update a report cron job**.

    ```POST``` to ``` /report/settings/:teamID ``` with a json payload in the body like this:

```
 {
     "str" : "* * * * * *"
 }
```
    This will create a new cron job that will schedule a report associated with the given team ID.

    If the team ID already exists it will replace the schedule with the new passed cron string.

* **Bulk set**.

  ```POST``` to ``` /report/entries/``` with a json payload in the body like this:

```
 [
     {
      "str" : "* * * * * *",
      "team_id":"a_team_id"
      "overwrite": true/false
     },
     {
      "str" : "* * * * * *",
      "team_id":"a_team_id"
      "overwrite": true/false
     }
 ]
```
    This will create a new report cron job for each item defined in the array only
    if no other schedule for the same program exists, unless the 'overwrite' param
    is set to true (default if omitted in the payload is false), in that case the
    existent job is overwritten

* **Delete a schedule**.

    ```DELETE``` to: ``` /report/entries/:teamID ``` .

    The end point will return 200 if the entry was deleted and 400 if the entry was not found.

# Docker execute

Those are the variables you have to use:

|Variable|Description|Sample|
|---|---|---|
|PORT||8081|
|AWS_REGION||eu-west-1|
|AWS_S3_ENDPOINT|AWS SDK S3 endpoint|http://localhost:9000|
|PATH_STYLE|Access bucket through path instead hostname |false|
|CRONTINUOUS_BUCKET||vulcan-crontinuous-local-bucket|
|VULCAN_API||http://localhost:8080/api|
|VULCAN_USER|User to interact with Vulcan API when creating scans|vulcan-scheduler@vulcan.com|
|VULCAN_TOKEN|Vulcan API authorization token|TOKEN|
|ENABLE_TEAMS_WHITELIST_SCAN|Flag to enable whitelist on scan scheduling|false|
|TEAMS_WHITELIST_SCAN|List of whitelisted team IDs for scan scheduling|[]|
|ENABLE_TEAMS_WHITELIST_REPORT|Flag to enable whitelist on report scheduling|false|
|TEAMS_WHITELIST_REPORT|List of whitelisted team IDs for report scheduling|[]|

```bash
docker build . -t vc

# Use the default config.toml customized with env variables.
docker run --env-file ./local.env vc

# Use custom config.toml
docker run -v `pwd`/custom.toml:/app/config.toml vc
```
