FROM node:10
ENV ENVIRONMENT=prod
WORKDIR /usr/src/app/code
COPY . /usr/src/app/code
RUN ls
RUN npm install
EXPOSE 9988
CMD ["node", "index.js"]