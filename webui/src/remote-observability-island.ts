(function () {
  'use strict';

  const React = window.React;
  const ReactDOM = window.ReactDOM;
  if (!React || !ReactDOM || typeof ReactDOM.createRoot !== 'function') {
    return;
  }

  const h = React.createElement;
  const roots = new WeakMap();

  function getRoot(container) {
    if (!container) return null;
    let root = roots.get(container);
    if (!root) {
      root = ReactDOM.createRoot(container);
      roots.set(container, root);
    }
    return root;
  }

  function SummaryCards(props) {
    const cards = Array.isArray(props.cards) ? props.cards : [];
    const children = cards.map((card, cardIndex) => {
      const lines = Array.isArray(card.lines) ? card.lines : [];
      const cardChildren = [
        h('h4', { key: 'title' }, String(card.title || '')),
      ];
      lines.forEach((line, lineIndex) => {
        cardChildren.push(
          h(
            'div',
            { className: 'instance-meta', key: 'line-' + String(lineIndex) },
            String(line || ''),
          ),
        );
      });
      return h(
        'div',
        { className: 'agent-card', key: 'card-' + String(cardIndex) },
        cardChildren,
      );
    });
    return h(React.Fragment, null, children);
  }

  function OperationRows(props) {
    const rows = Array.isArray(props.rows) ? props.rows : [];
    if (!rows.length) {
      return h(
        'tr',
        { key: 'empty' },
        h(
          'td',
          {
            colSpan: 6,
            className: 'text-dim',
          },
          String(props.emptyMessage || 'No remote operation metrics yet.'),
        ),
      );
    }

    const children = rows.map((row, rowIndex) => {
      const name = String(row && row.name ? row.name : '');
      const values = [
        name,
        String(row && row.total != null ? row.total : ''),
        String(row && row.success != null ? row.success : ''),
        String(row && row.failure != null ? row.failure : ''),
        String(row && row.successRate != null ? row.successRate : ''),
        String(row && row.avgLatency != null ? row.avgLatency : ''),
      ];
      const cells = values.map((value, cellIndex) => h('td', { key: 'cell-' + String(cellIndex) }, value));
      return h('tr', { key: 'row-' + String(rowIndex) + '-' + name }, cells);
    });
    return h(React.Fragment, null, children);
  }

  function renderSummary(container, cards) {
    const root = getRoot(container);
    if (!root) return false;
    root.render(h(SummaryCards, { cards: cards }));
    return true;
  }

  function renderOperations(container, payload) {
    const root = getRoot(container);
    if (!root) return false;
    const rows = payload && Array.isArray(payload.rows) ? payload.rows : [];
    const emptyMessage = payload && payload.emptyMessage ? payload.emptyMessage : '';
    root.render(h(OperationRows, { rows: rows, emptyMessage: emptyMessage }));
    return true;
  }

  window.CarrierRemoteObservabilityIsland = {
    renderSummary,
    renderOperations,
  };
})();
