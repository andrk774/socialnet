// Минимальные типы для той части Leaflet 1.9, что использует socialnet.
// Написаны вручную (не из @types/leaflet), чтобы не тянуть пакет в сборку.
export type LatLngTuple = [number, number];
export type LatLngBoundsTuple = [LatLngTuple, LatLngTuple];

export interface Layer {
  addTo(map: LeafletMap | LayerGroup): this;
  remove(): this;
  bindTooltip(content: HTMLElement, options?: { direction?: string; permanent?: boolean; sticky?: boolean }): this;
}
export interface Path extends Layer { setStyle(style: PathOptions): this }
export interface PathOptions {
  color?: string; weight?: number; opacity?: number; fillColor?: string; fillOpacity?: number;
  radius?: number; className?: string; interactive?: boolean; lineJoin?: string; lineCap?: string;
}
export interface LayerGroup extends Layer { clearLayers(): this }
export interface AttributionControl { setPrefix(prefix: string | false): this; addAttribution(text: string): this }
export interface LeafletMap {
  setView(center: LatLngTuple, zoom: number): this;
  fitBounds(bounds: LatLngBoundsTuple, options?: { padding?: [number, number]; maxZoom?: number }): this;
  invalidateSize(): this;
  remove(): this;
  attributionControl: AttributionControl;
  on(type: "click", fn: (e: { latlng: { lat: number; lng: number } }) => void): this;
}
export interface MapOptions {
  zoomControl?: boolean; attributionControl?: boolean; scrollWheelZoom?: boolean | "center";
  dragging?: boolean; touchZoom?: boolean; doubleClickZoom?: boolean; boxZoom?: boolean; keyboard?: boolean;
  maxZoom?: number; minZoom?: number; preferCanvas?: boolean;
}
export interface TileLayerOptions { maxZoom?: number; minZoom?: number; attribution?: string; crossOrigin?: boolean | string }

export function map(el: HTMLElement, options?: MapOptions): LeafletMap;
export function tileLayer(url: string, options?: TileLayerOptions): Layer;
export function polyline(points: LatLngTuple[], options?: PathOptions): Path;
export function circleMarker(at: LatLngTuple, options?: PathOptions): Path;
export function layerGroup(): LayerGroup;
export const version: string;
