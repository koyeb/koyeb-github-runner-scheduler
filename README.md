# koyeb-github-runner-executor

koyeb-github-runner-executor is a golang HTTP server accepting requests from GitHub webhooks, and starting GitHub runners on demand on Koyeb.

# How to use

## Start an instance of koyeb-github-runner-executor

### Using the control panel

* On the Koyeb control panel, create a new service and select the "GitHub" deployment method
* Under "Public GitHub repository", enter the URL of this repository: https://github.com/koyeb/koyeb-github-runner-executor
* Select the "Dockerfile" builder
* Set the following environment variables:
    - **PORT:** 8000. Make sure this value matches the port exposed under the section "Exposing gyour service".
    - **KOYEB_TOKEN:** a token created from [the console](https://app.koyeb.com/user/settings/api) which will be used to create Koyeb instances dynamically. Prefer using a secret over a plain text environment variable.
    - **API_SECRET:** a random secret used to authenticate requests from GitHub webhooks. Prefer using a secret over a plain text environment variable, and keep this value for later.

### Using the CLI

XXX