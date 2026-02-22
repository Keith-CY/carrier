"use strict";

const fs = require("node:fs");

function parseKanbanConfig(raw, source = "kanban-config") {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new Error(`Invalid JSON in ${source}: ${error.message}`);
  }

  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(`Invalid kanban config in ${source}: expected an object`);
  }

  if (!Array.isArray(parsed.fields)) {
    throw new Error(`Invalid kanban config in ${source}: "fields" must be an array`);
  }

  if (parsed.projectId !== undefined && typeof parsed.projectId !== "string") {
    throw new Error(`Invalid kanban config in ${source}: "projectId" must be a string when provided`);
  }

  return parsed;
}

function loadKanbanConfig(configPath) {
  const raw = fs.readFileSync(configPath, "utf8");
  return parseKanbanConfig(raw, configPath);
}

module.exports = {
  loadKanbanConfig,
  parseKanbanConfig,
};
