// /home/ab/ws-pooler/tests/load/load_test.js
import ws from "k6/ws";
import { check, sleep } from "k6";
import { Counter } from "k6/metrics";

// Configuration
const wsUrl = "ws://localhost:8080/ws";
const sessionDuration = 60000; // Sessions will last for 60 seconds
const messageInterval = 3; // seconds between messages

// Custom Metrics
const wsConnectionErrors = new Counter("ws_connection_errors");

export const options = {
  // Define stages for a test targeting 500 concurrent connections.
  stages: [
    { duration: "2m", target: 500 }, // Ramp up to 500 users over 2 minutes
    { duration: "5m", target: 500 }, // Hold at 500 users for 5 minutes to observe stability
    { duration: "1m", target: 0 }, // Ramp down gracefully
  ],
  thresholds: {
    ws_session_duration: [`p(95)<${sessionDuration + 2000}`],
    ws_connection_errors: ["rate<0.01"], // Allow for a <1% connection error rate
  },
};

export default function () {
  const res = ws.connect(wsUrl, {}, function (socket) {
    let intervalID;

    // ---- On Connection Open ----
    socket.on("open", () => {
      // Set a timeout to cleanly close the connection
      socket.setTimeout(() => {
        // Stop the sender before closing to prevent race conditions
        clearInterval(intervalID);
        socket.close();
      }, sessionDuration);

      // Start the message sending loop and store its ID
      intervalID = socket.setInterval(() => {
        const payload = JSON.stringify({
          action: "echo",
          timestamp: Date.now(),
          vu: __VU,
        });
        socket.send(payload);
      }, messageInterval * 1000);
    });

    // ---- On Message Received ----
    socket.on("message", (data) => {
      check(data, {
        "is a valid JSON": (d) => {
          try {
            JSON.parse(d);
            return true;
          } catch (e) {
            return false;
          }
        },
      });
    });

    // ---- On Connection Close ----
    socket.on("close", () => {
      // This is expected behavior.
    });

    // ---- On Error ----
    socket.on("error", function (e) {
      wsConnectionErrors.add(1);
      console.error(`VU ${__VU}: An error occurred: ${e.error()}`);
    });
  });

  check(res, {
    "WebSocket connection successful": (r) => r && r.status === 101,
  });

  if (!res || res.status !== 101) {
    wsConnectionErrors.add(1);
  } else {
    // Hold the VU active for the duration of the session.
    sleep(sessionDuration / 1000 + 20);
  }
}
