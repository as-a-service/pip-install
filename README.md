# `pip install` as a service

A server that performs `pip install` and returns a zip file containing the installed Python packages (site-packages).

## Building and Running with Docker

Build the Docker image:
```bash
docker build . -t pip-install
```

Run the container:
```bash
docker run -p 8080:8080 pip-install
```

## Deploying to Google Cloud Run and proxying locally

Deploy:

```bash
gcloud run deploy pip-install \
  --source . \
  --cpu 8 \
  --memory 32Gi \
  --no-allow-unauthenticated \
  --region europe-west1
```

Expose the service locally using Cloud Run's local proxy:

```bash
gcloud run services proxy pip-install \
  --region europe-west1 \
  --port 8080
```

## Usage

You can now send a POST request to `/install` using either a `JSON` body or by uploading files using `multipart/form-data`.

### Using local files (recommended)

```bash
curl -X POST http://localhost:8080/install \
  -F "requirements.txt=@example/requirements.txt" \
  --output python_packages.zip
```

The `constraints.txt` field is optional.

### Using JSON body

```bash
curl -X POST http://localhost:8080/install \
  -H "Content-Type: application/json" \
  -d '{
    "requirements.txt": "flask==2.2.5\nrequests==2.31.0"
  }' \
  --output python_packages.zip
```

The server will:
1. Create a temporary directory
2. Write the requirements files
3. Run `pip install` (with constraints if provided)
4. Zip the resulting `site-packages` directory
5. Stream the zip file back in the response
