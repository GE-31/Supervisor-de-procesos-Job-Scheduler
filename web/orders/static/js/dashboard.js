// Inicialización, coordinación de polling y eventos globales del dashboard.
// La lógica de renderizado vive en orders.js; la de validación de
// formulario, en forms.js; los toasts y confirmaciones, en notifications.js.
// Este módulo solo orquesta cuándo se llama a cada cosa.

import { api } from "./api.js";
import { state, setOrders, setStats, updateFilters, clearFilters } from "./state.js";
import { renderOrdersTable, renderStats, applyStatusChange, deleteSelectedOrder } from "./orders.js";
import { validateForm, updateEstimate, setFormBusy, clearFieldErrors } from "./forms.js";
import { toast } from "./notifications.js";

const POLL_INTERVAL_MS = Number(document.body.dataset.refreshSeconds || 5) * 1000;

let ordersLoading = false;
let checkingConnection = false;
let ordersTimer = null;
let connectionTimer = null;

function setOrdersControlsDisabled(disabled) {
  document.querySelector("#btn-submit-form").disabled = disabled;
  document.querySelectorAll("#orders-table-body .row-actions button").forEach((button) => {
    button.disabled = disabled;
  });
}

function setApiIndicator(online) {
  const dot = document.querySelector("#api-status-dot");
  const text = document.querySelector("#api-status-text");
  const banner = document.querySelector("#api-offline-banner");
  dot.classList.remove("online", "offline", "unknown");
  if (online === null) {
    dot.classList.add("unknown");
    text.textContent = "Comprobando orders-api…";
  } else if (online) {
    dot.classList.add("online");
    text.textContent = "orders-api conectado";
    banner.classList.add("hidden");
  } else {
    dot.classList.add("offline");
    text.textContent = "orders-api desconectado";
    banner.classList.remove("hidden");
  }
  document.querySelector("#api-last-check").textContent = new Date().toLocaleTimeString();
  setOrdersControlsDisabled(!online);
}

async function checkConnection() {
  if (checkingConnection) return;
  checkingConnection = true;
  try {
    await api.health();
    state.apiOnline = true;
  } catch {
    state.apiOnline = false;
  } finally {
    checkingConnection = false;
    setApiIndicator(state.apiOnline);
  }
}

async function refreshOrdersAndStats() {
  if (ordersLoading) return;
  ordersLoading = true;
  document.querySelector("#orders-spinner").classList.remove("hidden");
  document.querySelector("#stats-spinner").classList.remove("hidden");
  try {
    const [list, stats] = await Promise.all([api.listOrders(), api.stats()]);
    setOrders(list.data);
    setStats(stats);
    state.apiOnline = true;
    setApiIndicator(true);
    renderOrdersTable(refreshOrdersAndStats);
    renderStats();
  } catch (error) {
    state.apiOnline = false;
    setApiIndicator(false);
    toast(error.message, "error");
  } finally {
    ordersLoading = false;
    document.querySelector("#orders-spinner").classList.add("hidden");
    document.querySelector("#stats-spinner").classList.add("hidden");
  }
}

function startPolling() {
  stopPolling();
  connectionTimer = setInterval(checkConnection, POLL_INTERVAL_MS);
  ordersTimer = setInterval(refreshOrdersAndStats, POLL_INTERVAL_MS);
}

function stopPolling() {
  clearInterval(connectionTimer);
  clearInterval(ordersTimer);
}

function wireFilters() {
  const customerInput = document.querySelector("#f-search-customer");
  const productInput = document.querySelector("#f-search-product");
  const statusSelect = document.querySelector("#f-filter-status");
  const sortSelect = document.querySelector("#f-sort");

  const rerender = () => renderOrdersTable(refreshOrdersAndStats);

  customerInput.addEventListener("input", () => {
    updateFilters({ customer: customerInput.value });
    rerender();
  });
  productInput.addEventListener("input", () => {
    updateFilters({ product: productInput.value });
    rerender();
  });
  statusSelect.addEventListener("change", () => {
    updateFilters({ status: statusSelect.value });
    rerender();
  });
  sortSelect.addEventListener("change", () => {
    updateFilters({ sort: sortSelect.value });
    rerender();
  });
  document.querySelector("#btn-clear-filters").addEventListener("click", () => {
    clearFilters();
    customerInput.value = "";
    productInput.value = "";
    statusSelect.value = "";
    sortSelect.value = "date_desc";
    rerender();
  });
}

function wireForm() {
  const form = document.querySelector("#order-form");
  form.addEventListener("input", () => updateEstimate(form));
  form.addEventListener("reset", () => {
    setTimeout(() => {
      updateEstimate(form);
      clearFieldErrors();
    }, 0);
  });
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const { valid, values } = validateForm(form);
    if (!valid) return;
    setFormBusy(form, true);
    try {
      await api.createOrder(values);
      toast(`Pedido registrado para ${values.customer}`, "success");
      form.reset();
      updateEstimate(form);
      await refreshOrdersAndStats();
    } catch (error) {
      toast(error.message, "error");
    } finally {
      setFormBusy(form, false);
    }
  });
}

function wireModal() {
  document.querySelector("#btn-apply-status").addEventListener("click", () => {
    applyStatusChange(refreshOrdersAndStats);
  });
  document.querySelector("#btn-delete-order").addEventListener("click", () => {
    deleteSelectedOrder(refreshOrdersAndStats);
  });
}

function wireGlobalControls() {
  document.querySelector("#btn-check-connection").addEventListener("click", checkConnection);

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      checkConnection();
      refreshOrdersAndStats();
      startPolling();
    } else {
      stopPolling();
    }
  });
}

async function init() {
  wireFilters();
  wireForm();
  wireModal();
  wireGlobalControls();
  updateEstimate(document.querySelector("#order-form"));

  await checkConnection();
  await refreshOrdersAndStats();
  startPolling();
}

init();
