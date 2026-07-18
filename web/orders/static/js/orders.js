// Renderizado de la tabla de pedidos, el modal de detalle, el cambio de
// estado y la eliminación. Todo el HTML dinámico se construye con el DOM
// (createElement/textContent), nunca con innerHTML sobre datos de pedidos,
// para no arriesgar inyección si un campo de texto llega con caracteres
// especiales.

import { api } from "./api.js";
import { state, setOrders, setStats, filteredOrders } from "./state.js";
import { toast, confirmAction } from "./notifications.js";

const currencyFormatter = new Intl.NumberFormat("es-ES", { style: "currency", currency: "USD" });
const dateFormatter = new Intl.DateTimeFormat("es-ES", { dateStyle: "medium", timeStyle: "short" });

function formatCurrency(value) {
  return currencyFormatter.format(Number(value) || 0);
}

function formatDate(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : dateFormatter.format(date);
}

function statusBadge(status) {
  const span = document.createElement("span");
  span.className = `badge ${status}`;
  span.textContent = status;
  return span;
}

function actionButton(label, className, onClick, ariaLabel) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `button small ${className}`;
  button.textContent = label;
  if (ariaLabel) button.setAttribute("aria-label", ariaLabel);
  button.addEventListener("click", onClick);
  return button;
}

// renderOrdersTable dibuja las filas visibles según los filtros actuales del
// estado (state.filters), sin volver a solicitar nada a orders-api.
// onChanged se invoca tras una eliminación exitosa desde la tabla para
// refrescar pedidos y estadísticas de inmediato, sin esperar al polling.
export function renderOrdersTable(onChanged) {
  const tbody = document.querySelector("#orders-table-body");
  if (!tbody) return;
  tbody.textContent = "";

  const orders = filteredOrders();
  if (orders.length === 0) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.colSpan = 9;
    cell.className = "empty";
    cell.textContent = state.orders.length === 0 ? "No hay pedidos registrados." : "Ningún pedido coincide con los filtros.";
    row.append(cell);
    tbody.append(row);
    return;
  }

  for (const order of orders) {
    const row = document.createElement("tr");

    const cells = [order.id, order.customer, order.product, order.quantity, formatCurrency(order.unit_price), formatCurrency(order.total)];
    for (const value of cells) {
      const cell = document.createElement("td");
      cell.textContent = String(value);
      row.append(cell);
    }

    const statusCell = document.createElement("td");
    statusCell.append(statusBadge(order.status));
    row.append(statusCell);

    const dateCell = document.createElement("td");
    dateCell.textContent = formatDate(order.created_at);
    row.append(dateCell);

    const actionsCell = document.createElement("td");
    actionsCell.className = "row-actions";
    actionsCell.append(
      actionButton("Ver", "secondary", () => viewOrder(order.id), `Ver detalle del pedido ${order.id}`),
      actionButton("Eliminar", "danger", () => deleteOrder(order.id, onChanged), `Eliminar pedido ${order.id}`),
    );
    row.append(actionsCell);

    tbody.append(row);
  }
}

export function renderStats() {
  const stats = state.stats;
  const grid = document.querySelector("#stats-grid");
  if (!grid) return;
  const values = stats || { total_orders: "—", pending: "—", processing: "—", completed: "—", cancelled: "—", total_units: "—", total_amount: "—" };
  for (const [key, value] of Object.entries(values)) {
    const node = grid.querySelector(`[data-stat="${key}"]`);
    if (!node) continue;
    node.textContent = key === "total_amount" ? formatCurrency(value) : String(value);
  }
}

function detailRow(dl, label, value) {
  const dt = document.createElement("dt");
  dt.textContent = label;
  const dd = document.createElement("dd");
  dd.textContent = value;
  dl.append(dt, dd);
}

function renderOrderDetail(order) {
  const dl = document.querySelector("#order-detail-body");
  dl.textContent = "";
  detailRow(dl, "ID", String(order.id));
  detailRow(dl, "Cliente", order.customer);
  detailRow(dl, "Producto", order.product);
  detailRow(dl, "Cantidad", String(order.quantity));
  detailRow(dl, "Precio unitario", formatCurrency(order.unit_price));
  detailRow(dl, "Total", formatCurrency(order.total));
  detailRow(dl, "Estado", order.status);
  detailRow(dl, "Fecha", formatDate(order.created_at));

  const select = document.querySelector("#detail-status-select");
  select.value = order.status;
}

export async function viewOrder(id) {
  try {
    const cached = state.orders.find((o) => o.id === id);
    const order = cached || (await api.getOrder(id));
    state.selectedOrderId = order.id;
    renderOrderDetail(order);
    document.querySelector("#order-detail-modal").showModal();
  } catch (error) {
    toast(error.message, "error");
  }
}

// applyStatusChange aplica el estado elegido en el select del modal al
// pedido seleccionado, pidiendo confirmación si se cancela un pedido ya
// completado.
export async function applyStatusChange(onChanged) {
  const id = state.selectedOrderId;
  if (id == null) return;
  const select = document.querySelector("#detail-status-select");
  const newStatus = select.value;
  const current = state.orders.find((o) => o.id === id);

  if (current && current.status === "completed" && newStatus === "cancelled") {
    const confirmed = await confirmAction(`El pedido ${id} ya está completado. ¿Confirmas cancelarlo de todas formas?`);
    if (!confirmed) return;
  }

  try {
    await api.updateStatus(id, newStatus);
    toast(`Estado del pedido ${id} actualizado a "${newStatus}"`, "success");
    await onChanged();
  } catch (error) {
    toast(error.message, "error");
  }
}

export async function deleteSelectedOrder(onChanged) {
  const id = state.selectedOrderId;
  if (id == null) return;
  await deleteOrder(id, onChanged, true);
}

async function deleteOrder(id, onChanged, closeModal = false) {
  const confirmed = await confirmAction(`¿Confirmas eliminar el pedido ${id}? Esta acción no se puede deshacer.`);
  if (!confirmed) return;
  try {
    await api.deleteOrder(id);
    if (closeModal) document.querySelector("#order-detail-modal").close();
    toast(`Pedido ${id} eliminado`, "success");
    if (onChanged) await onChanged();
  } catch (error) {
    toast(error.message, "error");
  }
}

export { formatCurrency, formatDate, setOrders, setStats };
