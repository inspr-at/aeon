// SPDX-License-Identifier: AGPL-3.0-only
// Lazy-only renderer boundary. Both engines own their animation loop; dispose
// stops it, releases GPU resources/contexts and clears retained DOM/listeners.
import type { ForceGraph3DInstance } from '3d-force-graph'
import type ForceGraph2D from 'force-graph'
import type { Group, Mesh, MeshPhongMaterial, Object3D, Sprite, SpriteMaterial, Material, Texture, PerspectiveCamera } from 'three'
import type { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { endpointID, graphLayout, graphPalette, graphRadius, type GraphDimension, type KnowledgeGraphData, type LayoutEdge, type LayoutNode } from './knowledgeGraph'

type Graph3D = ForceGraph3DInstance<LayoutNode, LayoutEdge>
type Graph2D = ForceGraph2D<LayoutNode, LayoutEdge>
export interface GraphEmphasis { selected: string; neighbours: Set<string>; matches: Set<string>; searching: boolean; hovered: string }
export interface KnowledgeGraphRenderer {
  dimension: GraphDimension
  data(value: KnowledgeGraphData): void
  emphasis(value: GraphEmphasis): void
  theme(): void
  resize(width: number, height: number): void
  fit(): void
  focus(id: string): void
  motion(paused: boolean): void
  interact(): void
  dispose(): void
}
interface Options { reduced: boolean; signal: AbortSignal; select: (node: LayoutNode) => void; open: (node: LayoutNode) => void; hover: (node: LayoutNode | null) => void; clear: () => void }

export async function createKnowledgeGraphRenderer(host: HTMLElement, dimension: GraphDimension, options: Options): Promise<KnowledgeGraphRenderer | null> {
  let three: typeof import('three') | undefined
  let SpriteText: typeof import('three-spritetext').default | undefined
  let g3: Graph3D | undefined, g2: Graph2D | undefined, glowTexture: Texture | undefined
  let palette = graphPalette(), disposed = false, interacted = false, paused = options.reduced
  let emphasis: GraphEmphasis = { selected: '', neighbours: new Set(), matches: new Set(), searching: false, hovered: '' }
  let nodes: LayoutNode[] = [], links: LayoutEdge[] = []
  const objects = new Map<string, Group>()
  const labels = new Map<string, Sprite>()
  let labelNeighbours = new Set<string>()
  let paintFrame = 0, fitFrame = 0, settleTimer: ReturnType<typeof setTimeout> | undefined
  // Until someone drags, zooms or picks on the canvas, the camera keeps the whole graph in view:
  // after each load, once the layout settles, and when the stage changes size.
  let cameraTaken = false, fitOnSettle = true
  let pickTimer: ReturnType<typeof setTimeout> | undefined
  let pointerNode: LayoutNode | null = null
  let lastPick: { node: LayoutNode; x: number; y: number; at: number } | null = null
  let openedAt = -Infinity
  const duration = () => options.reduced || paused ? 0 : 650
  const active = (n: LayoutNode) => (!emphasis.selected || emphasis.neighbours.has(n.id)) && (!emphasis.searching || emphasis.matches.has(n.id))
  const labelled = (n: LayoutNode) => n.id === emphasis.hovered || n.id === emphasis.selected || (!!emphasis.selected && labelNeighbours.has(n.id))
  const touches = (l: LayoutEdge) => !!emphasis.selected && [endpointID(l.source), endpointID(l.target)].includes(emphasis.selected)
  const color = (n: LayoutNode) => palette.colors[n.type] ?? palette.muted
  const alpha = (n: LayoutNode) => active(n) ? 1 : .14
  const particles = (l: LayoutEdge) => !options.reduced && !paused && touches(l) ? 1 : 0
  const short = (n: LayoutNode) => n.title.length > 34 ? `${n.title.slice(0, 31)}…` : n.title
  const linkColor = (l: LayoutEdge) => {
    const rgb = palette.muted // Semantic palette values are opaque hex colours.
    const opacity = touches(l) ? 'aa' : emphasis.selected || emphasis.searching ? '18' : palette.dark ? '48' : '40'
    return /^#[\da-f]{6}$/i.test(rgb) ? rgb + opacity : rgb
  }
  const releaseObject = (object: Object3D) => object.traverse(part => {
    const mesh = part as Mesh
    mesh.geometry?.dispose()
    const materials = mesh.material ? Array.isArray(mesh.material) ? mesh.material : [mesh.material] : []
    for (const material of materials) {
      const map = (material as Material & { map?: Texture }).map
      if (map !== glowTexture) map?.dispose()
      material.dispose()
    }
  })
  // Screen size of a label's text line, in CSS pixels.
  const labelPx = (n: LayoutNode) => n.id === emphasis.selected ? 13.5 : 12.5
  const labelWeight = (n: LayoutNode) => n.id === emphasis.selected ? '600' : '500'
  // SpriteText draws its own canvas. Drawn at the size it shows on screen and
  // sampled without mipmaps, the text stays crisp instead of a washed-out blur.
  function styleLabel(text: InstanceType<typeof import('three-spritetext').default>, n: LayoutNode) {
    const px = labelPx(n), units = 5 / px // SpriteText units per CSS pixel (text height 5 = px)
    const want = {
      fontSize: Math.ceil(px * Math.min(window.devicePixelRatio || 1, 2)), fontWeight: labelWeight(n), fontFace: palette.font,
      color: palette.ink, backgroundColor: palette.label, borderColor: palette.line,
    }
    let changed = false
    for (const [key, value] of Object.entries(want) as [keyof typeof want, string & number][]) {
      if (text[key] !== value) { (text as unknown as Record<string, unknown>)[key] = value; changed = true }
    }
    if (text.userData.px !== px) {
      text.padding = [7 * units, 3 * units]; text.borderWidth = units; text.borderRadius = 11 * units
      text.userData.px = px; changed = true
    }
    if (changed) {
      const map = text.material.map
      if (map && three) { map.generateMipmaps = false; map.minFilter = three.LinearFilter; map.needsUpdate = true }
      text.userData.baseScale = text.scale.clone()
    }
  }
  function updateObjects() {
    if (!three || !SpriteText) return
    for (const n of nodes) {
      const group = objects.get(n.id); if (!group) continue
      const sphere = group.children[0] as Mesh
      const material = sphere.material as MeshPhongMaterial
      material.color.set(color(n)); material.opacity = alpha(n)
      material.emissive.set(palette.dark ? color(n) : 0x000000)
      material.emissiveIntensity = palette.dark && active(n) ? .15 : 0
      const glow = group.children[1] as Sprite
      glow.visible = palette.dark
      const glowMaterial = glow.material as SpriteMaterial
      glowMaterial.color.set(color(n)); glowMaterial.opacity = .12 * alpha(n)
      const wantLabel = labelled(n)
      let label = labels.get(n.id)
      if (wantLabel && !label) {
        label = new SpriteText(short(n), 5, palette.ink)
        const text = label as InstanceType<typeof SpriteText>
        text.material.depthTest = false; text.renderOrder = 10
        group.add(label); labels.set(n.id, label)
      } else if (!wantLabel && label) { group.remove(label); releaseObject(label); labels.delete(n.id); label = undefined }
      if (label) { const text = label as InstanceType<typeof SpriteText>; styleLabel(text, n); text.material.opacity = n.id === emphasis.hovered ? 1 : alpha(n) }
    }
    placeLabels()
  }
  type Box = { x: number; y: number; w: number; h: number }
  const overlaps = (a: Box, b: Box) => Math.abs(a.x - b.x) < (a.w + b.w) / 2 + 5 && Math.abs(a.y - b.y) < (a.h + b.h) / 2 + 4
  const labelNodes = () => nodes.filter(labelled).sort((a, b) =>
    Number(b.id === emphasis.selected) - Number(a.id === emphasis.selected) || Number(b.id === emphasis.hovered) - Number(a.id === emphasis.hovered) || b.degree - a.degree)
  function placeLabels() {
    if (!g3 || disposed) return
    const camera = g3.camera() as PerspectiveCamera, placed: Box[] = []
    for (const n of labelNodes()) {
      const label = labels.get(n.id); if (!label) continue
      const distance = Math.hypot(camera.position.x - (n.x ?? 0), camera.position.y - (n.y ?? 0), camera.position.z - (n.z ?? 0))
      const worldPerPixel = 2 * distance * Math.tan(camera.fov * Math.PI / 360) / Math.max(1, g3.height())
      // Scaling the sprite changes geometry only, avoiding a texture upload on
      // every camera frame. The text keeps its screen size at any zoom.
      label.scale.copy(label.userData.baseScale).multiplyScalar(worldPerPixel * labelPx(n) / 5)
      label.position.y = -graphRadius(n) - worldPerPixel * 17
      const screen = g3.graph2ScreenCoords(n.x ?? 0, (n.y ?? 0) + label.position.y, n.z ?? 0)
      const box = { x: screen.x, y: screen.y, w: label.scale.x / worldPerPixel, h: label.scale.y / worldPerPixel }
      label.visible = n.id === emphasis.selected || n.id === emphasis.hovered || !placed.some(b => overlaps(box, b))
      if (label.visible) placed.push(box)
    }
  }
  function draw2D(n: LayoutNode, ctx: CanvasRenderingContext2D, scale: number, picking?: string) {
    const x = n.x ?? 0, y = n.y ?? 0, radius = graphRadius(n)
    ctx.globalAlpha = picking ? 1 : alpha(n)
    ctx.beginPath(); ctx.arc(x, y, radius, 0, Math.PI * 2)
    if (picking) { ctx.fillStyle = picking; ctx.fill(); return }
    ctx.fillStyle = color(n); ctx.fill()
    ctx.lineWidth = .65 / scale; ctx.strokeStyle = palette.background; ctx.stroke()
    if (n.id === emphasis.selected) { ctx.beginPath(); ctx.arc(x, y, radius + 3 / scale, 0, Math.PI * 2); ctx.strokeStyle = color(n); ctx.lineWidth = 1 / scale; ctx.stroke() }
    ctx.globalAlpha = 1
  }
  const pointerArea = (n: LayoutNode, color: string, ctx: CanvasRenderingContext2D, scale: number) => draw2D(n, ctx, scale, color)
  // The 2D engine repaints its hit canvas at most every 800ms. After the camera moves, repaint it once
  // the move is drawn (setting the paint flushes it), so a click lands on the bubble under the pointer.
  function refreshPicking(delay: number) {
    if (!g2) return
    clearTimeout(pickTimer)
    pickTimer = setTimeout(() => requestAnimationFrame(() => requestAnimationFrame(() => { if (!disposed) g2?.nodePointerAreaPaint(pointerArea) })), delay)
  }
  function labels2D(ctx: CanvasRenderingContext2D, scale: number) {
    const placed: Box[] = []
    for (const n of labelNodes()) {
      const x = n.x ?? 0, y = n.y ?? 0, radius = graphRadius(n)
      const font = labelPx(n) / scale, text = short(n), height = font + 6 / scale, ly = y + radius + (6 + height * scale / 2) / scale
      ctx.font = `${labelWeight(n)} ${font}px ${palette.font}`
      ctx.textAlign = 'center'; ctx.textBaseline = 'middle'
      const width = ctx.measureText(text).width + 14 / scale
      const box = { x: x * scale, y: ly * scale, w: width * scale, h: height * scale }
      if (n.id !== emphasis.selected && n.id !== emphasis.hovered && placed.some(b => overlaps(box, b))) continue
      placed.push(box)
      ctx.globalAlpha = n.id === emphasis.hovered ? 1 : alpha(n)
      ctx.beginPath()
      if (ctx.roundRect) ctx.roundRect(x - width / 2, ly - height / 2, width, height, height / 2)
      else ctx.rect(x - width / 2, ly - height / 2, width, height)
      ctx.fillStyle = palette.label; ctx.fill()
      ctx.lineWidth = 1 / scale; ctx.strokeStyle = palette.line; ctx.stroke()
      ctx.fillStyle = palette.ink; ctx.fillText(text, x, ly + .5 / scale)
    }
    ctx.globalAlpha = 1
  }
  function rotation() {
    if (!g3) return
    const controls = g3.controls() as OrbitControls
    controls.autoRotate = !interacted && !paused && !options.reduced
    controls.autoRotateSpeed = .12; controls.enableDamping = !options.reduced
  }
  // Pausing freezes forces/particles, while the renderer stays interactive for
  // panning and selection. Full pauseAnimation is reserved for disposal.
  function motion(value: boolean) {
    paused = options.reduced || value
    const graph = g3 ?? g2
    graph?.cooldownTicks(paused ? 0 : 140).linkDirectionalParticles(particles)
    if (!paused) graph?.d3ReheatSimulation()
    rotation(); redraw()
  }
  function redraw() {
    if (disposed) return
    updateObjects()
    const graph = g3 ?? g2
    graph?.linkColor(linkColor).linkDirectionalParticles(particles)
    // Canvas auto-pauses its idle redraw. Refresh accessors wake it without
    // restarting physics or changing coordinates.
    g2?.nodeCanvasObject((n, ctx, scale) => draw2D(n, ctx, scale))
  }
  if (dimension === '3d') {
    const modules = await Promise.all([import('3d-force-graph'), import('three'), import('three-spritetext')])
    if (options.signal.aborted) return null
    three = modules[1]; SpriteText = modules[2].default
    try {
      g3 = new modules[0].default(host, { controlType: 'orbit', rendererConfig: { antialias: true, alpha: true, powerPreference: 'low-power' } }) as unknown as Graph3D
      g3.renderer().setPixelRatio(Math.min(window.devicePixelRatio, 2))
      // A shared procedural halo adds only a faint dark-mode glow, without a
      // full-screen postprocess changing the theme's background or text colours.
      const glowCanvas = document.createElement('canvas'); glowCanvas.width = glowCanvas.height = 64
      const glowContext = glowCanvas.getContext('2d')!
      const gradient = glowContext.createRadialGradient(32, 32, 0, 32, 32, 32)
      gradient.addColorStop(0, '#ffffff'); gradient.addColorStop(.45, '#ffffff60'); gradient.addColorStop(1, '#ffffff00')
      glowContext.fillStyle = gradient; glowContext.fillRect(0, 0, 64, 64)
      glowTexture = new three.CanvasTexture(glowCanvas)
      g3.showNavInfo(false).backgroundColor(palette.background).nodeThreeObject(n => {
        const group = new three!.Group()
        const material = new three!.MeshPhongMaterial({ color: color(n), transparent: true, opacity: alpha(n), shininess: 48, emissive: palette.dark ? color(n) : 0x000000, emissiveIntensity: .15 })
        const sphere = new three!.Mesh(new three!.SphereGeometry(graphRadius(n), 20, 14), material)
        const glow = new three!.Sprite(new three!.SpriteMaterial({ map: glowTexture, color: color(n), opacity: .12 * alpha(n), depthWrite: false, blending: three!.AdditiveBlending }))
        glow.raycast = () => {} // Only the bubble participates in pointer picking.
        glow.scale.setScalar(graphRadius(n) * 4); glow.visible = palette.dark
        group.add(sphere, glow); objects.set(n.id, group)
        return group
      }).linkOpacity(1).linkWidth(.55)
      ;(g3.controls() as OrbitControls).addEventListener('change', placeLabels)
      rotation()
    } catch {
      // A denied/failed context is an ordinary capability fallback.
      const failedRenderer = g3?.renderer(); glowTexture?.dispose(); glowTexture = undefined; g3?._destructor(); failedRenderer?.forceContextLoss(); g3 = undefined
      host.querySelectorAll('canvas').forEach(canvas => { canvas.getContext('webgl2')?.getExtension('WEBGL_lose_context')?.loseContext() })
      host.replaceChildren()
    }
  }
  if (!g3) {
    const { default: ForceGraph } = await import('force-graph')
    if (options.signal.aborted) return null
    g2 = new ForceGraph<LayoutNode, LayoutEdge>(host)
    g2.backgroundColor(palette.background).nodeCanvasObject((n, ctx, scale) => draw2D(n, ctx, scale))
      .nodePointerAreaPaint(pointerArea).onRenderFramePost(labels2D).linkWidth(l => touches(l) ? 1 : .6)
  }
  const graph = (g3 ?? g2)!
  graph.nodeLabel(() => '').linkLabel(() => '').nodeRelSize(8).nodeVal(n => Math.pow(n.degree + 1, 1.5))
    .linkColor(linkColor).linkDirectionalParticles(particles).linkDirectionalParticleWidth(1.4).linkDirectionalParticleSpeed(.002)
    .warmupTicks(90).cooldownTicks(paused ? 0 : 140).d3VelocityDecay(.38)
    .onNodeClick((node, event) => {
      if (performance.now() - openedAt < 100) return
      lastPick = { node, x: event.clientX, y: event.clientY, at: performance.now() }
      options.select(node)
    }).onNodeHover(node => { pointerNode = node; options.hover(node) })
    .onBackgroundClick(() => { if (performance.now() - openedAt >= 100) options.clear() })
    .onNodeDrag(() => { interacted = true; rotation() }).onEngineTick(placeLabels)
    .onEngineStop(() => { if (fitOnSettle && !cameraTaken && !emphasis.selected) fit(); fitOnSettle = false })
  const takeCamera = () => { cameraTaken = true }
  host.addEventListener('pointerdown', takeCamera, { passive: true }); host.addEventListener('wheel', takeCamera, { passive: true })
  // Use the browser's double-click gesture, rather than timing callbacks that
  // the force engine defers to animation frames. Keep the first hit during its
  // camera flight so the moving bubble cannot escape the second click.
  function doubleClick(event: MouseEvent) {
    const recent = lastPick && performance.now() - lastPick.at < 600 && Math.hypot(event.clientX - lastPick.x, event.clientY - lastPick.y) < 8
    const node = pointerNode ?? (recent ? lastPick!.node : null)
    if (node) { event.preventDefault(); openedAt = performance.now(); options.open(node) }
  }
  host.addEventListener('dblclick', doubleClick)
  graph.d3Force('charge')?.strength(-60)
  // A gentle centre force gives disconnected knowledge a stable home instead
  // of letting charge push it arbitrarily far beyond its linked neighbours.
  graph.d3Force('knowledge-centre', (alpha: number) => {
    for (const n of nodes) {
      n.vx = (n.vx ?? 0) - (n.x ?? 0) * .07 * alpha
      n.vy = (n.vy ?? 0) - (n.y ?? 0) * .07 * alpha
      if (g3) n.vz = (n.vz ?? 0) - (n.z ?? 0) * .07 * alpha
    }
  })
  graph.d3Force('link')?.distance((l: LayoutEdge) => 45 + (typeof l.source === 'object' ? graphRadius(l.source) : 5) + (typeof l.target === 'object' ? graphRadius(l.target) : 5))
  // Fit fills about 80% of the stage along its tighter side, bubbles included.
  // No closer than 2.2 screen pixels per graph unit, so a few entries stay calm.
  const FILL = .8, MAX_ZOOM = 2.2
  function fit() {
    if (disposed || !nodes.length) return
    if (g3 && three) {
      const camera = g3.camera() as PerspectiveCamera
      const right = new three.Vector3(1, 0, 0).applyQuaternion(camera.quaternion)
      const up = new three.Vector3(0, 1, 0).applyQuaternion(camera.quaternion)
      const back = new three.Vector3(0, 0, 1).applyQuaternion(camera.quaternion)
      const span = (axis: import('three').Vector3) => {
        let low = Infinity, high = -Infinity
        for (const n of nodes) { const d = axis.x * (n.x ?? 0) + axis.y * (n.y ?? 0) + axis.z * (n.z ?? 0), r = graphRadius(n); low = Math.min(low, d - r); high = Math.max(high, d + r) }
        return (low + high) / 2
      }
      const centre = right.clone().multiplyScalar(span(right)).add(up.clone().multiplyScalar(span(up))).add(back.clone().multiplyScalar(span(back)))
      const tan = Math.tan(camera.fov * Math.PI / 360), aspect = Math.max(1, g3.width()) / Math.max(1, g3.height())
      let distance = g3.height() / (2 * tan * MAX_ZOOM)
      for (const n of nodes) {
        const v = new three.Vector3(n.x ?? 0, n.y ?? 0, n.z ?? 0).sub(centre), r = graphRadius(n), depth = v.dot(back)
        distance = Math.max(distance, depth + (Math.abs(v.dot(up)) + r) / (tan * FILL), depth + (Math.abs(v.dot(right)) + r) / (tan * aspect * FILL))
      }
      const at = centre.clone().add(back.multiplyScalar(distance))
      g3.cameraPosition({ x: at.x, y: at.y, z: at.z }, { x: centre.x, y: centre.y, z: centre.z }, duration())
    } else if (g2) {
      let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity
      for (const n of nodes) { const r = graphRadius(n), x = n.x ?? 0, y = n.y ?? 0; x0 = Math.min(x0, x - r); x1 = Math.max(x1, x + r); y0 = Math.min(y0, y - r); y1 = Math.max(y1, y + r) }
      const zoom = Math.min(MAX_ZOOM, g2.width() * FILL / Math.max(1, x1 - x0), g2.height() * FILL / Math.max(1, y1 - y0))
      g2.centerAt((x0 + x1) / 2, (y0 + y1) / 2, duration()); g2.zoom(zoom, duration()); refreshPicking(duration())
    }
  }
  function focus(id: string) {
      clearTimeout(settleTimer)
      const node = nodes.find(n => n.id === id); if (!node) return
      if (g3) {
        const x = node.x ?? 0, y = node.y ?? 0, z = node.z ?? 0
        const camera = g3.cameraPosition(), length = Math.hypot(camera.x - x, camera.y - y, camera.z - z) || 1
        const distance = 150 + graphRadius(node) * 6
        g3.cameraPosition({ x: x + (camera.x - x) / length * distance, y: y + (camera.y - y) / length * distance, z: z + (camera.z - z) / length * distance }, { x, y, z }, duration())
      } else { g2!.centerAt(node.x ?? 0, node.y ?? 0, duration()); g2!.zoom(1.8, duration()); refreshPicking(duration()) }
    }
  return {
    dimension: g3 ? '3d' : '2d',
    data(value) {
      pointerNode = null; lastPick = null; cameraTaken = false; fitOnSettle = true
      objects.forEach(releaseObject); objects.clear(); labels.clear()
      const layout = graphLayout(value); nodes = layout.nodes; links = layout.links
      graph.graphData({ nodes, links }); redraw()
      cancelAnimationFrame(paintFrame); clearTimeout(settleTimer)
      paintFrame = requestAnimationFrame(() => { redraw(); if (emphasis.selected) focus(emphasis.selected); else fit() })
      if (!paused) settleTimer = setTimeout(() => { if (emphasis.selected) focus(emphasis.selected); else fit() }, 1200)
    },
    emphasis(value) {
      emphasis = value
      // A hub may have hundreds of neighbours. Label the ten most connected
      // ones, plus the selection and hover; overlapping labels are skipped.
      labelNeighbours = new Set(nodes.filter(n => value.neighbours.has(n.id) && n.id !== value.selected)
        .sort((a, b) => b.degree - a.degree || a.id.localeCompare(b.id)).slice(0, 10).map(n => n.id))
      redraw()
    },
    theme() { palette = graphPalette(); graph.backgroundColor(palette.background); redraw() },
    resize(width, height) {
      graph.width(Math.max(1, width)).height(Math.max(1, height))
      cancelAnimationFrame(fitFrame)
      // In 2D the selection stays centred when the stage narrows for the preview pane;
      // the 3D camera already looks at it.
      const node = g2 && emphasis.selected ? nodes.find(n => n.id === emphasis.selected) : undefined
      if (node) fitFrame = requestAnimationFrame(() => { g2?.centerAt(node.x ?? 0, node.y ?? 0); refreshPicking(0) })
      else if (!cameraTaken && !emphasis.selected) fitFrame = requestAnimationFrame(fit)
    },
    fit,
    focus,
    motion,
    interact() { interacted = true; clearTimeout(settleTimer); rotation() },
    dispose() {
      if (disposed) return
      disposed = true; cancelAnimationFrame(paintFrame); cancelAnimationFrame(fitFrame); clearTimeout(settleTimer); clearTimeout(pickTimer)
      host.removeEventListener('dblclick', doubleClick); host.removeEventListener('pointerdown', takeCamera); host.removeEventListener('wheel', takeCamera)
      graph.onNodeHover(() => {}).onNodeClick(() => {}).onBackgroundClick(() => {}).onNodeDrag(() => {}).onEngineStop(() => {}).pauseAnimation()
      if (g3) (g3.controls() as OrbitControls).removeEventListener('change', placeLabels)
      const renderer = g3?.renderer()
      objects.forEach(releaseObject); objects.clear(); labels.clear()
      glowTexture?.dispose(); glowTexture = undefined
      graph._destructor()
      renderer?.forceContextLoss()
      nodes = []; links = []; host.replaceChildren()
    },
  }
}
