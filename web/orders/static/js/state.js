// Estado en memoria del dashboard: pedidos recuperados, estadísticas,
// filtros activos y estado de conexión. orders-web no persiste nada; este
// estado se reconstruye en cada polling desde orders-api.

export const state = {
  orders: [],
  stats: null,
  apiOnline: null, // null = aún no comprobado
  loadingOrders: false,
  loadingStats: false,
  selectedOrderId: null,
  filters: {
    customer: "",
    product: "",
    status: "",
    sort: "date_desc",
  },
};

export function setOrders(orders) {
  state.orders = Array.isArray(orders) ? orders : [];
}

export function setStats(stats) {
  state.stats = stats;
}

export function setApiOnline(online) {
  state.apiOnline = online;
}

export function updateFilters(partial) {
  Object.assign(state.filters, partial);
}

export function clearFilters() {
  state.filters.customer = "";
  state.filters.product = "";
  state.filters.status = "";
  state.filters.sort = "date_desc";
}

function matchesFilters(order, filters) {
  const customer = filters.customer.trim().toLowerCase();
  const product = filters.product.trim().toLowerCase();
  if (customer && !order.customer.toLowerCase().includes(customer)) return false;
  if (product && !order.product.toLowerCase().includes(product)) return false;
  if (filters.status && order.status !== filters.status) return false;
  return true;
}

function compareOrders(a, b, sort) {
  switch (sort) {
    case "date_asc":
      return new Date(a.created_at) - new Date(b.created_at);
    case "total_desc":
      return b.total - a.total;
    case "total_asc":
      return a.total - b.total;
    case "date_desc":
    default:
      return new Date(b.created_at) - new Date(a.created_at);
  }
}

// filteredOrders aplica búsqueda, filtro de estado y orden sobre los
// pedidos ya recuperados; no dispara ninguna solicitud nueva.
export function filteredOrders() {
  return state.orders
    .filter((order) => matchesFilters(order, state.filters))
    .sort((a, b) => compareOrders(a, b, state.filters.sort));
}
