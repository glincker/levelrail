<script setup lang="ts">
import { PhArrowLeft, PhCaretDown, PhCaretRight } from '@phosphor-icons/vue'
import MockStatusPill from './MockStatusPill.vue'
import { appNav, activeApp, brand, globalNav } from './mockData'
import { mockIcons } from './mockIcons'

defineProps<{ mode: 'global' | 'app'; active: string }>()
</script>

<template>
  <aside class="pm-side">
    <div class="pm-side__brand">
      <span class="pm-side__logo"><i></i><i></i><i></i></span>
      <span>
        <b>{{ brand.name }}</b>
        <small>{{ brand.sub }}</small>
      </span>
    </div>

    <template v-if="mode === 'global'">
      <section v-for="g in globalNav" :key="g.section">
        <div class="pm-side__sec"><span>{{ g.section }}</span><PhCaretDown :size="12" /></div>
        <div v-for="i in g.items" :key="i.id" class="pm-side__item" :class="{ 'is-active': i.id === active }">
          <component :is="mockIcons[i.icon]" :size="16" />{{ i.label }}
        </div>
      </section>
    </template>

    <template v-else>
      <div class="pm-side__item pm-side__back"><PhArrowLeft :size="16" />All apps</div>
      <div class="pm-side__app"><b>{{ activeApp }}</b><MockStatusPill status="healthy" /></div>
      <template v-for="i in appNav" :key="i.id">
        <div class="pm-side__item" :class="{ 'is-active': i.id === active }">
          <component :is="mockIcons[i.icon]" :size="16" />{{ i.label }}
          <component :is="i.children ? PhCaretDown : PhCaretRight" v-if="i.children || i.id === 'traffic' || i.id === 'config'" class="pm-side__caret" :size="12" />
        </div>
        <div v-for="c in i.children" :key="c.id" class="pm-side__item pm-side__sub" :class="{ 'is-active': c.id === active }">
          <component :is="mockIcons[c.icon]" :size="15" />{{ c.label }}
        </div>
      </template>
    </template>

    <div class="pm-side__user"><span>D</span>{{ brand.user }}</div>
  </aside>
</template>

<style scoped>
.pm-side {
  position: relative;
  flex: none;
  width: 240px;
  margin: 8px 0 8px 8px;
  padding: 8px;
  border-radius: 12px;
  background: var(--pm-surface);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-text);
  font-size: 13px;
}
.pm-side__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  margin-bottom: 6px;
  border-radius: 8px;
  box-shadow: 0 0 0 1px var(--pm-border);
}
.pm-side__brand b { display: block; color: var(--pm-head); font-size: 13px; line-height: 1.2; }
.pm-side__brand small { color: var(--pm-muted); font-size: 11.5px; }
.pm-side__logo {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 2px;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: #0e1a21;
}
.pm-side__logo i { width: 16px; height: 5px; border-radius: 2px; background: #2fb3dc; }
.pm-side__logo i:nth-child(2) { width: 12px; background: #1a8fb4; }
.pm-side__logo i:nth-child(3) { background: #107292; }
.pm-side__sec {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 12px 8px 4px;
  color: var(--pm-muted);
  font-size: 12px;
  font-weight: 500;
}
.pm-side__item {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 32px;
  padding: 0 10px;
  border-radius: 8px;
  color: var(--pm-head);
}
.pm-side__item.is-active { background: var(--pm-sunken); font-weight: 500; }
.pm-side__sub { margin-left: 12px; height: 28px; font-size: 12.5px; }
.pm-side__back { color: var(--pm-text); margin-top: 4px; }
.pm-side__caret { margin-left: auto; color: var(--pm-muted); }
.pm-side__app {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 8px;
  margin: 4px 0;
}
.pm-side__app b { color: var(--pm-head); font-size: 13.5px; }
.pm-side__user {
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: 10px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  color: var(--pm-head);
}
.pm-side__user span {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--pm-sunken);
  font-size: 10px;
}
</style>
