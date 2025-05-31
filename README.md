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

Send a POST request to `/install` with a JSON body containing your `package.json` content and optionally your `package-lock.json` content.

Example request:
```bash
curl -X POST http://localhost:8080/install \
  -H "Content-Type: application/json" \
  -d '{
    "package.json": "{\"name\":\"test-package\",\"dependencies\":{\"express\":\"^4.17.1\"}}",
    "package-lock.json": ""
  }' \
  --output npm_build.zip
```

The server will:
1. Create a temporary directory
2. Write the package files
3. Run `npm install` (or `npm ci` if package-lock.json is provided)
4. Zip the resulting `node_modules` directory
5. Stream the zip file back in the response
