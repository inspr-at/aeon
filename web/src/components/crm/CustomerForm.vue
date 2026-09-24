<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import '../../styles/crm.css'
import type { CustomerDraft, DraftProblems } from '../../lib/crm'
import AppIcon from '../AppIcon.vue'

// Edit mode's form: the whole customer in groups, one Save for all of it. The
// page owns the draft and saving; this only lays the fields out.
const props = defineProps<{ draft: CustomerDraft; problems: DraftProblems; touched: boolean }>()
const bad = (key: keyof DraftProblems) => props.touched && !!props.problems[key]
function copyBilling() { Object.assign(props.draft.visiting, { ...props.draft.billing }) }
const ADDRESS = [
  { key: 'street', label: 'Street', wide: true, auto: 'street-address', max: 500 },
  { key: 'postal_code', label: 'Postal code', wide: false, auto: 'postal-code', max: 50 },
  { key: 'city', label: 'City', wide: false, auto: 'address-level2', max: 200 },
  { key: 'country', label: 'Country', wide: true, auto: 'country-name', max: 100 },
] as const
</script>

<template>
  <div class="customer-form">
    <fieldset class="f-group">
      <legend>Customer</legend>
      <div class="f-grid">
        <div class="f-row wide">
          <label class="f-label" for="edit-name">Name</label>
          <input id="edit-name" v-model="draft.name" class="field name-field" maxlength="200" autocomplete="off" :aria-invalid="bad('name')" aria-describedby="edit-name-note" />
          <p v-if="bad('name')" id="edit-name-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.name }}</p>
        </div>
        <div class="f-row"><label class="f-label" for="edit-legal">Legal name</label><input id="edit-legal" v-model="draft.legal_name" class="field" maxlength="200" autocomplete="off" placeholder="As on invoices" /></div>
        <div class="f-row"><label class="f-label" for="edit-industry">Industry</label><input id="edit-industry" v-model="draft.industry" class="field" maxlength="200" autocomplete="off" /></div>
        <div class="f-row">
          <label class="f-label" for="edit-website">Website</label>
          <input id="edit-website" v-model="draft.website" class="field" type="url" inputmode="url" maxlength="500" autocomplete="off" placeholder="hofer.at" :aria-invalid="bad('website')" aria-describedby="edit-website-note" />
          <p v-if="bad('website')" id="edit-website-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.website }}</p>
        </div>
        <div class="f-row"><label class="f-label" for="edit-phone">Phone</label><input id="edit-phone" v-model="draft.phone" class="field" type="tel" maxlength="100" autocomplete="off" /></div>
        <div class="f-row"><label class="f-label" for="edit-domain">Email domain</label><input id="edit-domain" v-model="draft.domain" class="field" maxlength="255" autocomplete="off" /></div>
      </div>
    </fieldset>

    <fieldset class="f-group">
      <legend>Rates and size</legend>
      <div class="f-grid three">
        <div class="f-row">
          <label class="f-label" for="edit-currency">Currency</label>
          <input id="edit-currency" v-model="draft.currency" class="field mono" maxlength="3" autocomplete="off" placeholder="EUR" :aria-invalid="bad('currency')" aria-describedby="edit-currency-note" @input="draft.currency = draft.currency.toUpperCase()" />
          <p v-if="bad('currency')" id="edit-currency-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.currency }}</p>
        </div>
        <div class="f-row">
          <label class="f-label" for="edit-hourly">Hourly rate</label>
          <div class="f-affix"><input id="edit-hourly" v-model="draft.hourly" class="field mono" inputmode="decimal" autocomplete="off" placeholder="95.00" :aria-invalid="bad('hourly')" aria-describedby="edit-hourly-note" /><span class="affix" aria-hidden="true">/ h</span></div>
          <p id="edit-hourly-note" class="f-note" :class="{ bad: bad('hourly') }" :role="bad('hourly') ? 'alert' : undefined">{{ bad('hourly') ? problems.hourly : 'Quotes suggest it for this customer.' }}</p>
        </div>
        <div class="f-row">
          <label class="f-label" for="edit-lp">Rate per point</label>
          <div class="f-affix"><input id="edit-lp" v-model="draft.lp" class="field mono" inputmode="decimal" autocomplete="off" placeholder="120.00" :aria-invalid="bad('lp')" aria-describedby="edit-lp-note" /><span class="affix" aria-hidden="true">/ pt</span></div>
          <p v-if="bad('lp')" id="edit-lp-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.lp }}</p>
        </div>
        <div class="f-row">
          <label class="f-label" for="edit-employees">Employees</label>
          <input id="edit-employees" v-model="draft.employees" class="field mono" inputmode="numeric" autocomplete="off" :aria-invalid="bad('employees')" aria-describedby="edit-employees-note" />
          <p v-if="bad('employees')" id="edit-employees-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.employees }}</p>
        </div>
        <div class="f-row">
          <label class="f-label" for="edit-revenue">Annual revenue</label>
          <input id="edit-revenue" v-model="draft.revenue" class="field mono" inputmode="decimal" autocomplete="off" :aria-invalid="bad('revenue')" aria-describedby="edit-revenue-note" />
          <p v-if="bad('revenue')" id="edit-revenue-note" class="f-note bad" role="alert"><AppIcon name="alert" :size="12" />{{ problems.revenue }}</p>
        </div>
      </div>
    </fieldset>

    <fieldset class="f-group">
      <legend>Registration</legend>
      <div class="f-grid three">
        <div class="f-row"><label class="f-label" for="edit-vat">VAT ID</label><input id="edit-vat" v-model="draft.vat_id" class="field mono" maxlength="100" autocomplete="off" placeholder="ATU12345678" /></div>
        <div class="f-row"><label class="f-label" for="edit-tax">Tax number</label><input id="edit-tax" v-model="draft.tax_id" class="field mono" maxlength="100" autocomplete="off" /></div>
        <div class="f-row"><label class="f-label" for="edit-register">Company register</label><input id="edit-register" v-model="draft.register_no" class="field mono" maxlength="100" autocomplete="off" placeholder="FN 123456a" /></div>
      </div>
    </fieldset>

    <div class="addresses">
      <fieldset class="f-group">
        <legend>Billing address</legend>
        <div class="f-grid">
          <div v-for="field in ADDRESS" :key="field.key" class="f-row" :class="{ wide: field.wide }">
            <label class="f-label" :for="`billing-${field.key}`">{{ field.label }}</label>
            <input :id="`billing-${field.key}`" v-model="draft.billing[field.key]" class="field" :maxlength="field.max" :autocomplete="`billing ${field.auto}`" />
          </div>
        </div>
      </fieldset>
      <fieldset class="f-group">
        <legend>Visiting address</legend>
        <div class="f-grid">
          <div v-for="field in ADDRESS" :key="field.key" class="f-row" :class="{ wide: field.wide }">
            <label class="f-label" :for="`visiting-${field.key}`">{{ field.label }}</label>
            <input :id="`visiting-${field.key}`" v-model="draft.visiting[field.key]" class="field" :maxlength="field.max" autocomplete="off" />
          </div>
        </div>
        <button type="button" class="btn sm ghost copy" @click="copyBilling"><AppIcon name="copy" :size="13" />Same as billing</button>
      </fieldset>
    </div>

    <fieldset class="f-group">
      <legend>About and notes</legend>
      <div class="f-grid">
        <div class="f-row wide"><label class="f-label" for="edit-description">Description</label><textarea id="edit-description" v-model="draft.description" class="field" rows="3" maxlength="20000" placeholder="What they do, in a sentence or two" /></div>
        <div class="f-row wide"><label class="f-label" for="edit-notes">Notes <span class="opt">for your team, Markdown</span></label><textarea id="edit-notes" v-model="draft.customer_notes" class="field notes-field" rows="5" maxlength="20000" /></div>
      </div>
    </fieldset>
  </div>
</template>

<style scoped>
.customer-form { display: grid; gap: 18px; }
.name-field { height: 42px !important; font-size: 16px !important; font-weight: 600; }
.addresses { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 28px; padding-top: 18px; border-top: 1px solid var(--line); }
.addresses .f-group + .f-group { padding-top: 0; border-top: 0; }
.addresses + .f-group { padding-top: 18px; border-top: 1px solid var(--line); }
.copy { justify-self: start; }
.notes-field { font-family: var(--mono); font-size: 13px; font-variant-ligatures: none; }
@media (max-width: 900px) { .addresses { grid-template-columns: minmax(0, 1fr); } .addresses .f-group + .f-group { padding-top: 18px; border-top: 1px solid var(--line); } }
</style>
