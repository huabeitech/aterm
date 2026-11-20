import test from 'node:test'
import assert from 'node:assert/strict'
import {compareAllHosts, hostSortScope, formFromHost, formToInput, validateForm} from '../src/renderer/src/features/hosts/model/hostModel'
import {fixtureHost} from './support/fixtures'

test('All Hosts retains recent ordering and has no manual scope', () => {
  const hosts=[fixtureHost('old',{lastUsedAt:'2024-01-01'}),fixtureHost('unused'),fixtureHost('recent',{lastUsedAt:'2026-01-01'})]
  assert.deepEqual(hosts.sort(compareAllHosts).map(host=>host.alias),['recent','old','unused'])
  assert.equal(hostSortScope('all',null),null)
})
test('host form validation reports required fields and invalid port without transport models', () => {
  const form={...formFromHost(fixtureHost('valid')),alias:'bad alias',hostName:'',port:'70000'}
  const errors=validateForm(form)
  assert.deepEqual(Object.keys(errors).sort(),['alias','hostName','port'])
  assert.ok(Object.values(errors).every(value=>typeof value==='string'&&value.length>0))
})
test('host input maps numeric defaults and never repopulates a saved password', () => {
  const form={...formFromHost(fixtureHost('saved',{passwordAuth:true})),port:'',connectTimeout:''}
  assert.equal(form.password,'')
  assert.equal(form.passwordState,'saved')
  const input=formToInput(form)
  assert.equal(input.port,22)
  assert.equal(input.connectTimeout,0)
  assert.equal(input.password,'')
})
