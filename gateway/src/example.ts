import { safeHandleCommand } from "./index";

const example = "telegram 123 req-1 /agents";
const response = safeHandleCommand(example);
console.log(JSON.stringify(response, null, 2));
