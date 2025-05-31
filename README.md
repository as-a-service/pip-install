# `npm install` as a service

A server that performs `npm install` and returns a zip file containing the `node_modules`.

## Building and Running with Docker

Build the Docker image:
```bash
docker build . -t npm-install
```

Run the container:
```bash
docker run -p 8080:8080 npm-install
```

## Deploying to Google Cloud Run and proxying locally

Deploy:

```bash
gcloud run deploy npm-install \
  --source . \
  --cpu 8 \
  --memory 32Gi \
  --no-allow-unauthenticated \
  --region europe-west1
```

Expose the service locally using Cloud Run's local proxy:

```bash
gcloud run services proxy npm-install \
  --region europe-west1 \
  --port 8080
```

## Usage

You can now send a POST request to `/install` using either a `JSON` body or by uploading files using `multipart/form-data`.

### Using local files (recommended)

```bash
curl -X POST http://localhost:8080/install \
  -F "package.json=@example/package.json" \
  -F "package-lock.json=@example/package-lock.json" \
  --output node_modules.zip
```

The `package-lock.json` field is optional.

### Using JSON body

```bash
curl -X POST http://localhost:8080/install \
  -H "Content-Type: application/json" \
  -d '{
    "package.json": "{\"name\":\"test-package\",\"dependencies\":{\"express\":\"^4.17.1\"}}"
  }' \
  --output node_modules.zip
```

The server will:
1. Create a temporary directory
2. Write the package files
3. Run `npm install` (or `npm ci` if package-lock.json is provided)
4. Zip the resulting `node_modules` directory
5. Stream the zip file back in the response
