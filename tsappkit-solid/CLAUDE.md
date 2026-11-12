# tsappkit-solid/

`@panyam/tsappkit-solid`: `SolidIsland`, `signalView`, `useView`.

- Tests run against a published tsappkit, its devDependency (`^0.1.0`), not the source beside it, so new core API isn't importable here until a release. Type against it structurally (`useView` takes any `{ attach }`) and use a small local stand-in in tests. Importing `../../tsappkit/src` fails `tsc` with TS6059.
- The peer on tsappkit is `>=0.0.5 <1.0.0`. A caret on 0.x pins too tightly (`^0.1.0` means 0.1.x only), so keep it a plain range, or every tsappkit minor needs a solid release.
- Solid's `render` appends to its container, which is why `SolidIsland.activate` clears its element first (#39).
