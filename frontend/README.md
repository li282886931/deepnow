# DeepNow Admin Console

## H5 smoke test

Install the Python Playwright browser once if Chromium is not available:

```bash
python3 -m playwright install chromium
```

Start the H5 server in one terminal:

```bash
cd frontend
npm run dev:h5 -- --host 127.0.0.1
```

Run the Python Playwright smoke test in a second terminal:

```bash
cd frontend
npm run test:e2e
```

To target another local H5 server:

```bash
cd frontend
npm run test:e2e -- --base-url http://127.0.0.1:5173
```

The test mocks `/api/**` requests in the browser and does not write fake data to
the application or production database. It covers login, overview, model,
API Key, and knowledge navigation at `1440x900`, `1024x768`, and `390x844`,
including a horizontal-overflow check.

## Unit tests and build

```bash
cd frontend
npm run test:unit
npm run build:h5
```

## Single-binary delivery

Build the H5 console, copy its required assets into the Go embed directory, and
compile the server as one binary:

```bash
cd frontend && npm run build:h5 && cd .. && mkdir -p src/webdist && cp frontend/dist/build/h5/index.html src/webdist/index.html && cp -R frontend/dist/build/h5/assets src/webdist/ && CGO_ENABLED=0 go build -o deepnow ./src
```

The server embeds `src/webdist/` and serves the console from the same binary as
the API. Upstream secrets are supplied by the backend and are not committed to
frontend environment files.
