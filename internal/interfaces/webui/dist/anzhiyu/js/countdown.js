let CountdownTimer=(()=>{let cfg=window.GLOBAL_CONFIG&&GLOBAL_CONFIG.countdown||{},config={targetDate:cfg.targetDate||"2027-01-01",targetName:cfg.targetName||"元旦",units:{day:{text:"今日",unit:"小时"},week:{text:"本周",unit:"天"},month:{text:"本月",unit:"天"},year:{text:"本年",unit:"天"}}},calculators={day:()=>{var hours=(new Date).getHours();return{remaining:24-hours,percentage:hours/24*100}},week:()=>{var day=(new Date).getDay(),day=0===day?6:day-1;return{remaining:6-day,percentage:(1+day)/7*100}},month:()=>{var now=new Date,total=new Date(now.getFullYear(),now.getMonth()+1,0).getDate(),now=now.getDate()-1;return{remaining:total-now,percentage:now/total*100}},year:()=>{var now=new Date,start=new Date(now.getFullYear(),0,1),total=365+(now.getFullYear()%4==0?1:0),now=Math.floor((now-start)/864e5);return{remaining:total-now,percentage:now/total*100}}};function updateCountdown(){var eventDate,daysUntil,countRight,now,target,elements=["eventName","eventDate","daysUntil","countRight"].map(id=>document.getElementById(id));elements.some(el=>!el)||([elements,eventDate,daysUntil,countRight]=elements,now=new Date,target=new Date(config.targetDate),elements.textContent=config.targetName,eventDate.textContent=config.targetDate,daysUntil.textContent=Math.round((target-now.setHours(0,0,0,0))/864e5),countRight.innerHTML=Object.entries(config.units).map(([key,{text,unit}])=>{var{remaining:key,percentage}=calculators[key]();return`
                    <div class="cd-count-item">
                        <div class="cd-item-name">${text}</div>
                        <div class="cd-item-progress">
                            <div class="cd-progress-bar" style="width: ${percentage}%; opacity: ${percentage/100}"></div>
                            <span class="cd-percentage ${46<=percentage?"cd-many":""}">${percentage.toFixed(2)}%</span>
                            <span class="cd-remaining ${60<=percentage?"cd-many":""}">
                                <span class="cd-tip">还剩</span>${key}<span class="cd-tip">${unit}</span>
                            </span>
                        </div>
                    </div>
                `}).join(""))}let timer,start=()=>{var styleSheet;(styleSheet=document.createElement("style")).textContent=`
            .card-countdown .item-content {
                display: flex;
            }
            .cd-count-left {
                position: relative;
                display: flex;
                flex-direction: column;
                margin-right: 0.8rem;
                line-height: 1.5;
                align-items: center;
                justify-content: center;
            }
            .cd-count-left .cd-text {
                font-size: 14px;
            }
            .cd-count-left .cd-name {
                font-weight: bold;
                font-size: 18px;
            }
            .cd-count-left .cd-time {
                font-size: 30px;
                font-weight: bold;
                color: var(--anzhiyu-main);
            }
            .cd-count-left .cd-date {
                font-size: 12px;
                opacity: 0.6;
            }
            .cd-count-left::after {
                content: "";
                position: absolute;
                right: -0.8rem;
                width: 2px;
                height: 80%;
                background-color: var(--anzhiyu-main);
                opacity: 0.5;
            }
            .cd-count-right {
                flex: 1;
                margin-left: .8rem;
                display: flex;
                flex-direction: column;
                justify-content: space-between;
            }
            .cd-count-item {
                display: flex;
                flex-direction: row;
                align-items: center;
                height: 24px;
            }
            .cd-item-name {
                font-size: 14px;
                margin-right: 0.8rem;
                white-space: nowrap;
            }
            .cd-item-progress {
                position: relative;
                display: flex;
                flex-direction: row;
                align-items: center;
                justify-content: space-between;
                height: 100%;
                width: 100%;
                border-radius: 8px;
                background-color: var(--anzhiyu-background);
                overflow: hidden;
            }
            .cd-progress-bar {
                height: 100%;
                border-radius: 8px;
                background-color: var(--anzhiyu-main);
            }
            .cd-percentage,
            .cd-remaining {
                position: absolute;
                font-size: 12px;
                margin: 0 6px;
                transition: opacity 0.3s ease-in-out, transform 0.3s ease-in-out;
            }
            .cd-many {
                color: #fff;
            }
            .cd-remaining {
                opacity: 0;
                transform: translateX(10px);
            }
            .card-countdown .item-content:hover .cd-remaining {
                transform: translateX(0);
                opacity: 1;
            }
            .card-countdown .item-content:hover .cd-percentage {
                transform: translateX(-10px);
                opacity: 0;
            }
        `,document.head.appendChild(styleSheet),updateCountdown(),timer=setInterval(updateCountdown,6e5)};return["pjax:complete","DOMContentLoaded"].forEach(event=>document.addEventListener(event,start)),document.addEventListener("pjax:send",()=>timer&&clearInterval(timer)),{start:start,stop:()=>timer&&clearInterval(timer)}})();
