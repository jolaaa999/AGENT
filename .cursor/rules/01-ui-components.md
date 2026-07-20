# UI Component Rules

## Design System

整体参考：

Apple

Linear

Notion

Arc Browser

避免：

Element Plus 默认风格

Ant Design 后台风格

若依

AdminLTE

---

# Color

Primary

Indigo

Secondary

Purple

Success

Green

Warning

Orange

Danger

Red

Background

#F8FAFC

Surface

White

Border

Gray-200

Hover

Gray-100

禁止：

高饱和颜色。

超过6种主题色。

---

# Radius

Button

12px

Input

12px

Card

18px

Dialog

20px

Tooltip

12px

保持统一。

---

# Shadow

默认：

shadow-sm

Hover：

shadow-md

Dialog：

shadow-lg

不要重阴影。

---

# Typography

Title

32

Section

24

Card

18

Body

16

Caption

13

字体：

Inter

HarmonyOS Sans

字重：

400

500

600

避免：

Bold Everywhere

---

# Button

Primary

纯色

Secondary

描边

Ghost

透明

Danger

红色

按钮高度：

44px

圆角：

12px

Hover：

轻微变亮。

不要：

渐变按钮。

发光按钮。

---

# Card

所有信息都放在 Card。

统一：

Padding

24px

Radius

18px

Shadow

Small

Hover：

轻微上浮。

---

# Input

统一高度：

44px

Placeholder：

Secondary Text

Focus：

Primary Border

不要：

彩色边框。

---

# Sidebar

宽：

280px

背景：

White

Border Right

支持：

Collapse

Search

Tree

---

# Graph Panel

背景：

深色

占页面最大空间。

支持：

Zoom

Pan

MiniMap

Search

Node Highlight

Graph 永远保持第一视觉层级。

---

# AI Chat

像：

ChatGPT

NotebookLM

消息：

圆角

留白

Markdown

Code Highlight

Streaming

不要：

QQ聊天。

微信聊天。

---

# Loading

采用：

Skeleton

Progress

Streaming

AI Loading：

逐步展示。

不要：

Loading...

---

# Empty State

必须：

Icon

Title

Description

Action

禁止：

空白。

---

# Toast

成功：

Green

失败：

Red

警告：

Orange

位置：

右上角。

自动消失。

---

# Dialog

宽：

640~800

圆角：

20px

背景：

White

不要：

全屏弹窗。

---

# Tables

尽量减少。

如果必须：

圆角

Hover

Sticky Header

不要：

传统后台表格。

---

# Motion

Hover

200ms

Click

150ms

Dialog

250ms

Graph

Spring

禁止：

复杂动画。

---

# Icons

统一：

Lucide

不要混用：

Iconfont

阿里图标

Material

Emoji

---

# Responsive

Desktop First

Laptop

Tablet

Mobile

Graph 在小屏可折叠。

---

# Accessibility

所有按钮：

Hover

Focus

Disabled

Loading

必须全部实现。

---

# Component Principle

组件必须：

可复用

可组合

低耦合

不允许：

页面直接复制组件代码。

一个组件负责一件事。

UI 与业务逻辑分离。