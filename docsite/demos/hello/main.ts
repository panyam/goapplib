// The placeholder demo: proves a demo's bundle loads in its frame and can mark itself ready.
import { ready } from "../_lib/frame";

const msg = document.getElementById("msg")!;
const button = document.getElementById("count") as HTMLButtonElement;
let clicks = 0;
button.addEventListener("click", () => {
  clicks++;
  button.textContent = `Clicked ${clicks} time${clicks === 1 ? "" : "s"}`;
});
msg.textContent = "This frame's script loaded and ran.";
button.hidden = false;
ready();
