<script setup>
import SimpleToast from "@/components/SimpleToast.vue";
import { captcha, login } from "@/apis/index";
import router from "@/router";
import {
  clearSavedCredentials,
  credentialOperationVersion,
  credentialPersistenceNotice,
  loadSavedCredentials,
  persistSuccessfulLogin,
  setCredentialPreference,
} from "@/utils/saved-credentials";

const toastRef = ref();

const INIT_DATA = {
  account: "",
  password: "",
  code: "",
};
const fromData = ref({ ...INIT_DATA });
const remember = ref(false);
const credentialWarning = ref("");
const inputRevision = ref(0);
const rememberRevision = ref(0);
const captchaData = ref({
  id: "",
  data: "",
});

const getCaptcha = async () => {
  try {
    const response = await captcha();
    captchaData.value = response;
  } catch (e) {
    toastRef.value.show(e.message);
  }
};

const onCredentialInput = () => {
  inputRevision.value += 1;
};

const onRememberChange = (event) => {
  remember.value = event.target.checked;
  rememberRevision.value += 1;
  const preference = setCredentialPreference(remember.value);
  if (remember.value) return;

  fromData.value.password = "";
  void clearSavedCredentials(preference.version, preference.markerWritten)
    .then((result) => {
      if (!result.versionIsCurrent) return;
      if (result.complete) return;
      if (!result.savedLoginCleared) {
        credentialPersistenceNotice.value = result.futureAutofillBlocked
          ? "无法删除旧登录记录；已停用密码自动回填。请在浏览器设置中清除此站点数据。"
          : "无法删除旧登录记录，也无法保证重启后不恢复。请在浏览器设置中清除此站点数据。";
      } else if (!result.credentialsCleared) {
        credentialPersistenceNotice.value = result.futureAutofillBlocked
          ? "加密记录未能完全清除；已停用密码自动回填。"
          : "加密记录无法删除，且停用状态未能保存；无法保证重启后不恢复。";
      } else {
        credentialPersistenceNotice.value = "部分本机登录数据未能清除；已停用密码自动回填。";
      }
    })
    .catch(() => {
      if (credentialOperationVersion() !== preference.version) return;
      credentialPersistenceNotice.value = preference.markerWritten
        ? "清除操作未完成；自动回填已停用，但本机记录可能仍保留。"
        : "本机存储不可写，无法保证下次启动不会恢复保存的密码。";
    });
};

const LogIn = async () => {
  if (!fromData.value.account) {
    toastRef.value.show("请输入账号");
    return;
  } else if (!fromData.value.password) {
    toastRef.value.show("请输入密码");
    return;
  }
  const submittedAccount = fromData.value.account;
  const submittedPassword = fromData.value.password;
  try {
    await login({
      ...fromData.value,
      account: submittedAccount,
      password: submittedPassword,
      codeId: captchaData.value.id,
    });
  } catch (e) {
    getCaptcha();
    fromData.value.code = "";
    toastRef.value.show(e.message);
    return;
  }

  try {
    localStorage.removeItem("token");
  } catch {
    // Cookie-based authentication does not depend on the legacy token marker.
  }

  const shouldRemember = remember.value;
  const operation = credentialOperationVersion();
  void router.replace({ path: "/" });

  if (shouldRemember) {
    void persistSuccessfulLogin(
      submittedAccount,
      submittedPassword,
      operation
    ).then((result) => {
      if (credentialOperationVersion() !== operation) return;
      if (result.status === "failed" || result.notice) {
        credentialPersistenceNotice.value = result.notice || "本机凭据保存未完成。";
      }
    }).catch(() => {
      if (credentialOperationVersion() !== operation) return;
      credentialPersistenceNotice.value = "登录成功，但本机未能安全保存账号密码。";
    });
  }
};

// 清除输入内容
const clearInput = (field) => {
  fromData.value[field] = "";
  if (field === "account" || field === "password") onCredentialInput();
};

onMounted(() => {
  try {
    localStorage.removeItem("token");
  } catch {
    // Cookie-based authentication does not depend on the legacy token marker.
  }

  let savedLogin = null;
  try {
    savedLogin = localStorage.getItem("savedLogin");
  } catch {
    credentialWarning.value = "无法读取浏览器保存的账号；仍可手动登录。";
  }
  if (savedLogin) {
    try {
      const parsed = JSON.parse(savedLogin);
      if (typeof parsed.account === "string") {
        fromData.value.account = parsed.account;
        // Preserve legacy account recall, but do not consent to password storage.
        remember.value = false;
        try {
          localStorage.setItem("savedLogin", JSON.stringify({ account: parsed.account }));
        } catch {
          try {
            localStorage.removeItem("savedLogin");
          } catch {
            credentialWarning.value = "无法清理旧版登录记录；请在浏览器网站数据设置中清除该站点数据。";
          }
        }
      } else {
        localStorage.removeItem("savedLogin");
      }
    } catch {
      try {
        localStorage.removeItem("savedLogin");
      } catch {
        credentialWarning.value = "无法清理旧版登录记录；请在浏览器网站数据设置中清除该站点数据。";
      }
    }
  }

  const inputAtStart = inputRevision.value;
  const rememberAtStart = rememberRevision.value;
  const operationAtStart = credentialOperationVersion();
  void loadSavedCredentials()
    .then((saved) => {
      if (saved.status === "loaded") {
        if (
          inputRevision.value !== inputAtStart ||
          rememberRevision.value !== rememberAtStart ||
          credentialOperationVersion() !== operationAtStart
        ) return;
        fromData.value.account = saved.account;
        fromData.value.password = saved.password;
        remember.value = true;
        setCredentialPreference(true);
      } else if (saved.status === "unavailable") {
        credentialWarning.value = "无法读取安全保存的密码；请手动输入。";
      }
    })
    .catch(() => {
      credentialWarning.value = "无法读取安全保存的密码；请手动输入。";
    });
  getCaptcha();
});
</script>

<template>
  <div class="page f-r f-aic f-jcc">
    <div class="content f-c f-aic">
      <div class="title">账号密码登录</div>
      <div class="input-container">
        <div class="input">
          <input
            v-model="fromData.account"
            @input="onCredentialInput"
            placeholder="请输入账号"
            placeholder-class="input-placeholder"
            autofocus
          />
          <span
            v-if="fromData.account"
            class="clear-btn"
            @click="clearInput('account')"
            >×</span
          >
        </div>
      </div>
      <div class="input-container">
        <div class="input">
          <input
            v-model="fromData.password"
            @input="onCredentialInput"
            placeholder="请输入密码"
            placeholder-class="input-placeholder"
            type="password"
          />
          <span
            v-if="fromData.password"
            class="clear-btn"
            @click="clearInput('password')"
            >×</span
          >
        </div>
      </div>
      <div class="input-container f-r f-aic f-jcsb">
        <div class="input">
          <input
            v-model="fromData.code"
            placeholder="请输入验证码"
            placeholder-class="input-placeholder"
          />
          <span
            v-if="fromData.code"
            class="clear-btn"
            @click="clearInput('code')"
            >×</span
          >
        </div>
        <img :src="captchaData.data" @click="getCaptcha" />
      </div>

      <div class="remember-me-container">
        <input
          type="checkbox"
          id="rememberMe"
          :checked="remember"
          @change="onRememberChange"
        />
        <label for="rememberMe">记住账号和密码</label>
        <p v-if="credentialWarning" role="status">{{ credentialWarning }}</p>
      </div>

      <div class="button f-r f-jcc" @click="LogIn">登录</div>
    </div>
    <SimpleToast ref="toastRef" />
  </div>
</template>

<style scoped lang="less">
.page {
  font-size: 14px;
  height: 100dvh;
  width: 100dvw;
  .content {
    margin-top: -100px;
    width: 300px;
    .title {
      font-size: 20px;
      margin-bottom: 60px;
    }
    .input-container {
      width: 100%;
      position: relative;
      margin-top: 12px; /* 调整了间隔 */
      .input {
        width: 100%;
        background: #363636;
        position: relative;
        input {
          font-size: 14px;
          width: 100%;
          padding: 15px 10px;
          outline: none;
          border: none;
          background: #363636;
          color: white;
          padding-right: 30px; /* 为清除按钮留出空间 */
        }
        .clear-btn {
          position: absolute;
          right: 10px;
          top: 50%;
          transform: translateY(-50%);
          color: #999;
          font-size: 20px;
          cursor: pointer;
          &:hover {
            color: #fff;
          }
        }
      }
    }

    /* 新增：记住账号密码容器样式 */
    .remember-me-container {
      width: 100%; /* 与输入框对齐 */
      margin-top: 15px;
      font-size: 13px; /* 字体大小可以稍小 */
      color: #ccc; /* 颜色可以稍浅 */

      input[type="checkbox"] {
        margin-right: 5px; /* 调整复选框与文字的间隔 */
        // 你可以在这里添加更多复选框的自定义样式，
        // 但浏览器默认样式通常已足够
      }

      label {
        cursor: pointer; /* 让标签文字也能点击选中复选框 */
      }
    }

    .button {
      margin-top: 100px; /* 调整了与上方元素的间隔 */
      color: white;
      background-color: #363636;
      height: 40px;
      line-height: 40px;
      width: 100%;
      font-size: 14px;
      text-align: center; /* 确保文字居中 */
      cursor: pointer; /* 添加鼠标手势 */
    }
  }
}
</style>
