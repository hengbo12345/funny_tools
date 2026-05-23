import { elements, addConsoleLine, pollStatus } from './app.js';
import { connectSSE } from './api.js';

export function showTokenModal() {
	elements.tokenModal.classList.remove('hidden');
	elements.tokenInput.focus();
}

export function hideTokenModal() {
	elements.tokenModal.classList.add('hidden');
	elements.tokenError.classList.add('hidden');
	elements.tokenInput.value = '';
}

export async function verifyAndSubmitToken() {
	const token = elements.tokenInput.value.trim();
	if (!token) {
		elements.tokenError.textContent = "⚠️ 请输入您的访问 Token 凭证！";
		elements.tokenError.classList.remove('hidden');
		return;
	}

	elements.btnTokenSubmit.disabled = true;
	elements.btnTokenSubmit.textContent = "正在验证...";

	try {
		// Probe /api/status using the candidate token
		const response = await fetch('/api/status', {
			headers: {
				'Authorization': `Bearer ${token}`
			}
		});

		if (response.status === 401) {
			throw new Error("Token 校验未通过");
		}

		if (!response.ok) {
			throw new Error("服务器响应异常，请稍后再试");
		}

		// Success! Save token and initialize app state
		localStorage.setItem('gps_sim_token', token);
		hideTokenModal();
		
		// Reload status & logs with the correct credentials
		pollStatus();
		connectSSE();
		
		addConsoleLine("🔓 安全访问校验成功，设备管理面板已成功解锁！", "success");
	} catch (error) {
		elements.tokenError.textContent = `⚠️ 认证失败：${error.message === "Token 校验未通过" ? "Token 错误或已失效！" : error.message}`;
		elements.tokenError.classList.remove('hidden');
		elements.tokenInput.focus();
	} finally {
		elements.btnTokenSubmit.disabled = false;
		elements.btnTokenSubmit.textContent = "验证并进入控制台";
	}
}
