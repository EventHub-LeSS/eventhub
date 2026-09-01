const express = require("express");
const swaggerUi = require("swagger-ui-express");

const app = express();
const PORT = 8080;

const OPENAPI_URL =
  "https://www.keycloak.org/docs-api/latest/rest-api/openapi.json";

async function loadSpec() {
  const res = await fetch(OPENAPI_URL);
  if (!res.ok) throw new Error(`Failed to fetch spec: ${res.status}`);
  return res.json();
}

loadSpec()
  .then((spec) => {
    app.use("/", swaggerUi.serve, swaggerUi.setup(spec));

    app.listen(PORT, () => {
      console.log(`Keycloak Swagger UI running on port ${PORT}`);
    });
  })
  .catch((err) => {
    console.error("Could not load OpenAPI spec:", err.message);
    process.exit(1);
  });
