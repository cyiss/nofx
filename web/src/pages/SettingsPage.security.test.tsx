import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SettingsPage } from './SettingsPage'

const { logout } = vi.hoisted(() => ({ logout: vi.fn() }))
vi.mock('../contexts/AuthContext', () => ({ useAuth: () => ({user:{email:'user@example.test'}, logout}) }))
vi.mock('../contexts/LanguageContext', () => ({useLanguage: () => ({language:'en'})}))
vi.mock('../lib/api', () => ({api:{}}))
vi.mock('../components/trader/ExchangeConfigModal', () => ({ExchangeConfigModal:()=>null}))
vi.mock('../components/trader/TelegramConfigModal', () => ({TelegramConfigModal:()=>null}))
vi.mock('../components/trader/ModelConfigModal', () => ({ModelConfigModal:()=>null}))
vi.mock('sonner', () => ({toast:{error:vi.fn(),success:vi.fn()}}))

afterEach(()=>{vi.unstubAllGlobals();vi.clearAllMocks()})
describe('password security',()=>{
  it('requires the current password and signs out after changing it',async()=>{
    const request=vi.fn().mockResolvedValue({ok:true})
    vi.stubGlobal('fetch',request)
    render(<SettingsPage />)
    fireEvent.change(screen.getByLabelText('Current Password'),{target:{value:'old-password'}})
    fireEvent.change(screen.getByLabelText('New Password'),{target:{value:'new-password'}})
    fireEvent.click(screen.getByRole('button',{name:'Update Password'}))
    await waitFor(()=>expect(logout).toHaveBeenCalledOnce())
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({old_password:'old-password',new_password:'new-password'})
  })
  it('keeps the user signed in when the server rejects the current password',async()=>{
    vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,json:async()=>({error:'Invalid current password'})}))
    render(<SettingsPage />)
    fireEvent.change(screen.getByLabelText('Current Password'),{target:{value:'wrong-password'}})
    fireEvent.change(screen.getByLabelText('New Password'),{target:{value:'new-password'}})
    fireEvent.click(screen.getByRole('button',{name:'Update Password'}))
    await waitFor(()=>expect(screen.getByRole('button',{name:'Update Password'})).not.toBeDisabled())
    expect(logout).not.toHaveBeenCalled()
  })
})
