// Cliente HTTP del navegador hacia orders-web. Todas las solicitudes van a
// rutas /proxy/* de orders-web, nunca directamente a orders-api, para evitar
// CORS y centralizar el manejo de errores y timeouts en un solo lugar.

const REQUEST_TIMEOUT_MS = 8000;

class ApiError extends Error {
  constructor(status, code, message) {
    super(message || `Error HTTP ${status}`);
    this.status = status;
    this.code = code;
  }
}

async function request(path, options = {}) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  try {
    const response = await fetch(path, {
      ...options,
      signal: controller.signal,
      headers: {
        Accept: "application/json",
        ...(options.headers || {}),
      },
    });
    let body = null;
    const text = await response.text();
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = null;
      }
    }
    if (!response.ok) {
      throw new ApiError(response.status, body?.error || "unknown_error", body?.message);
    }
    return body;
  } catch (error) {
    if (error.name === "AbortError") {
      throw new ApiError(0, "timeout", "La solicitud tardó demasiado en responder");
    }
    if (error instanceof ApiError) {
      throw error;
    }
    throw new ApiError(0, "network_error", "No se pudo contactar con orders-web");
  } finally {
    clearTimeout(timeoutId);
  }
}

function jsonBody(payload) {
  return {
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  };
}

export const api = {
  health: () => request("/proxy/health"),
  listOrders: () => request("/proxy/orders"),
  createOrder: (payload) => request("/proxy/orders", { method: "POST", ...jsonBody(payload) }),
  getOrder: (id) => request(`/proxy/orders/${encodeURIComponent(id)}`),
  updateStatus: (id, status) =>
    request(`/proxy/orders/${encodeURIComponent(id)}/status`, { method: "PATCH", ...jsonBody({ status }) }),
  deleteOrder: (id) => request(`/proxy/orders/${encodeURIComponent(id)}`, { method: "DELETE" }),
  stats: () => request("/proxy/orders/stats"),
};

export { ApiError };
