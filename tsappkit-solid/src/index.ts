// @panyam/tsappkit-solid — Solid adapter for @panyam/tsappkit.
//
// Framework reactivity lives here so tsappkit core stays framework-neutral:
// SolidIsland bridges a Solid root into the tsappkit component lifecycle, and
// signalView backs a typed view-interface method with a Solid signal.
export { SolidIsland } from './SolidIsland';
export { signalView } from './signalView';
