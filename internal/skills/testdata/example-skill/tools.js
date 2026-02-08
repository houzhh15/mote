// Example skill tools for integration testing

function greet(params) {
  return {
    message: "Hello, " + params.name + "! Welcome!"
  };
}

function calculate(params) {
  var a = params.a;
  var b = params.b;
  var result;
  
  switch (params.op) {
    case "add":
      result = a + b;
      break;
    case "sub":
      result = a - b;
      break;
    case "mul":
      result = a * b;
      break;
    case "div":
      if (b === 0) {
        return { error: "division by zero" };
      }
      result = a / b;
      break;
    default:
      return { error: "unknown operation" };
  }
  
  return { result: result };
}

function beforeMessage(ctx) {
  // Log that the hook was called
  return {
    continue: true,
    modified: false
  };
}

module.exports = {
  greet: greet,
  calculate: calculate,
  beforeMessage: beforeMessage
};
