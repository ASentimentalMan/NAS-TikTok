<script setup>
import SimpleToast from "@/components/SimpleToast.vue";
import { captcha, login } from "@/apis/index";
import router from "@/router";

const toastRef = ref();

const INIT_DATA = {
  account: "",
  password: "",
  code: "",
};
const fromData = ref({ ...INIT_DATA });
const remember = ref(false);
const captchaData = ref({
  id: "",
  data: "",
});

const getCaptcha = async () => {
  try {
    const response = await captcha(fromData.value);
    captchaData.value = response;
  } catch (e) {
    toastRef.value.show(e.message);
  }
};

const LogIn = async () => {
  if (!fromData.value.account) {
    toastRef.value.show("请输入账号");
    return;
  } else if (!fromData.value.password) {
    toastRef.value.show("请输入密码");
    return;
  }
  try {
    const response = await login({
      ...fromData.value,
      codeId: captchaData.value.id,
    });
    localStorage.setItem("token", response.token);
    router.replace({ path: "/" });
  } catch (e) {
    getCaptcha();
    fromData.value.code = "";
    toastRef.value.show(e.message);
  }
  if (remember.value) {
    localStorage.setItem(
      "savedLogin",
      JSON.stringify({
        account: fromData.value.account,
        password: fromData.value.password,
      })
    );
  }
};

// 清除输入内容
const clearInput = (field) => {
  fromData.value[field] = "";
};

onMounted(() => {
  const savedLogin = localStorage.getItem("savedLogin");
  if (savedLogin) {
    remember.value = true;
    fromData.value = { ...JSON.parse(savedLogin), code: "" };
  }
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
        <input type="checkbox" id="rememberMe" v-model="remember" />
        <label for="rememberMe">记住账号</label>
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
