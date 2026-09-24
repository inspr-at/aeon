<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onMounted, onUnmounted, watch, ref, nextTick } from 'vue'
import type { QuoteDocumentData } from '../../../lib/quotes/types'
import type { PresenceSnapshot } from '../../../lib/quotePresence'
import { anchorFidelity, collaboratorColor } from '../../../lib/quotePresence'
const props=defineProps<{ root:HTMLElement|null; document:QuoteDocumentData; revision:number; principalId:string; presence:PresenceSnapshot|null }>()
interface Mark { id:string; name:string; fidelity:'section'|'precise'; color:string; left:number; top:number; width:number; height:number }
const marks=ref<Mark[]>([])
let resize:ResizeObserver|null=null,mutations:MutationObserver|null=null,frame=0,generation=0
function textPoint(host:Element,offset:number):{node:Node;offset:number}|null {
  const walker=document.createTreeWalker(host,NodeFilter.SHOW_TEXT);let remaining=offset;let node:Node|null
  while((node=walker.nextNode())){const length=node.textContent?.length??0;if(remaining<=length)return {node,offset:remaining};remaining-=length}
  return null
}
function recompute(){cancelAnimationFrame(frame);const current=++generation;frame=requestAnimationFrame(()=>{void draw(current)})}
async function draw(current:number){
  const root=props.root;if(!root){marks.value=[];return}
  const next:Mark[]=[]
  for(const s of props.presence?.sessions??[]){if(s.principal_id===props.principalId || !s.anchor || s.mode==='idle')continue
    const fidelity=await anchorFidelity(s.anchor,props.document,props.revision);if(current!==generation)return;if(fidelity==='missing')continue
    const section=root.querySelector(`[data-section-id="${CSS.escape(s.anchor.section_id)}"]`);if(!section)continue
    let rect=section.getBoundingClientRect();let width=rect.width,height=rect.height,kind:'section'|'precise'='section'
    if(fidelity==='precise' && s.anchor.node_id){const text=section.querySelector(`[data-text-id="${CSS.escape(s.anchor.node_id)}"]`)
      if(text){const point=textPoint(text,s.anchor.focus??0);if(point){const range=document.createRange();range.setStart(point.node,point.offset);range.collapse(true)
        const caret=range.getClientRects()[0];if(caret){rect=caret;width=2;height=Math.max(14,caret.height);kind='precise'}}}
    }
    next.push({id:s.session_id,name:s.name,fidelity:kind,color:collaboratorColor(s.principal_id),left:rect.left,top:rect.top,width,height})
  }
  if(current===generation)marks.value=next
}
function observe(){resize?.disconnect();mutations?.disconnect();if(props.root){resize=new ResizeObserver(recompute);resize.observe(props.root);mutations=new MutationObserver(recompute);mutations.observe(props.root,{childList:true,subtree:true,attributes:true})}recompute()}
onMounted(()=>{window.addEventListener('resize',recompute);window.addEventListener('scroll',recompute,true);void nextTick(observe)})
onUnmounted(()=>{generation++;resize?.disconnect();mutations?.disconnect();cancelAnimationFrame(frame);window.removeEventListener('resize',recompute);window.removeEventListener('scroll',recompute,true)})
watch(()=>props.root,observe);watch(()=>[props.presence,props.document,props.revision],recompute)
</script>
<template>
  <div class="quote-presence-overlays" aria-hidden="true">
    <div v-for="mark in marks" :key="mark.id" class="mark" :class="mark.fidelity" :style="{left:`${mark.left}px`,top:`${mark.top}px`,width:`${mark.width}px`,height:`${mark.height}px`,'--mark-color':mark.color}">
      <span class="label">{{ mark.name }}{{ mark.fidelity==='section' ? ' editing this section' : '' }}</span>
    </div>
  </div>
</template>
<style scoped>
.quote-presence-overlays{position:fixed;inset:0;z-index:15;pointer-events:none}.mark{position:fixed;box-sizing:border-box;border-radius:4px;outline:1px solid var(--mark-color);outline-offset:2px}.mark.section{background:transparent}.mark.precise{width:2px;background:var(--mark-color);outline:0;border-radius:0}.label{position:absolute;left:0;top:-24px;max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;padding:3px 6px;border-radius:5px;background:var(--mark-color);color:var(--surface);font-size:11px;font-weight:700}
@media print{.quote-presence-overlays{display:none}}
</style>
